package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coingecko/coingecko-cli/internal/api"
	"github.com/coingecko/coingecko-cli/internal/config"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// newTestWSServer creates a test WebSocket server that runs the given handler per connection.
func newTestWSServer(t *testing.T, handler func(conn *websocket.Conn)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		handler(conn)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func paidCfg() *config.Config {
	return &config.Config{APIKey: "test-key", Tier: "paid"}
}

func demoCfg() *config.Config {
	return &config.Config{APIKey: "test-key", Tier: "demo"}
}

func sendJSON(conn *websocket.Conn, v any) {
	data, _ := json.Marshal(v)
	_ = conn.WriteMessage(websocket.TextMessage, data)
}

// happyHandshake sends welcome, reads subscribe, sends CoinGecko-style confirm, reads set_tokens.
func happyHandshake(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	sendJSON(conn, map[string]string{"type": "welcome"})

	// Read subscribe command.
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err)
	var cmd actionCableCommand
	require.NoError(t, json.Unmarshal(raw, &cmd))
	assert.Equal(t, "subscribe", cmd.Command)

	// Send CoinGecko-style subscription confirmation.
	sendJSON(conn, map[string]any{"code": 2000, "message": "Subscription is successful"})

	// Read set_tokens message.
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)
}

func TestConnect_HappyPath(t *testing.T) {
	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		happyHandshake(t, conn)

		// Send a bare price update (CoinGecko format).
		sendJSON(conn, wsPayload{
			CoinID:    "bitcoin",
			Price:     45000.50,
			PricePct:  2.5,
			MarketCap: 850e9,
			Volume:    30e9,
			Timestamp: time.Now().Unix(),
		})

		// Keep connection open until client disconnects.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	client := NewClient(paidCfg(), []string{"bitcoin"})
	client.SetURL(wsURL(srv))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	updates, err := client.Connect(ctx)
	require.NoError(t, err)

	update := <-updates
	require.NotNil(t, update)
	assert.Equal(t, "bitcoin", update.CoinID)
	assert.Equal(t, 45000.50, update.Price)
	assert.Equal(t, 2.5, update.Change24h)

	require.NoError(t, client.Close())
}

func TestConnect_DemoPlanRejected(t *testing.T) {
	client := NewClient(demoCfg(), []string{"bitcoin"})
	_, err := client.Connect(context.Background())
	require.ErrorIs(t, err, api.ErrPlanRestricted)
}

func TestParseMessage_PriceUpdate(t *testing.T) {
	client := NewClient(paidCfg(), []string{"bitcoin"})

	// CoinGecko sends bare payload (not wrapped in ActionCable envelope).
	raw, _ := json.Marshal(wsPayload{
		CoinID:    "ethereum",
		Price:     3200.75,
		PricePct:  -1.2,
		MarketCap: 380e9,
		Volume:    15e9,
		Timestamp: 1700000000,
	})
	update := client.parseMessage(raw)

	require.NotNil(t, update)
	assert.Equal(t, "ethereum", update.CoinID)
	assert.Equal(t, 3200.75, update.Price)
	assert.Equal(t, -1.2, update.Change24h)
	assert.Equal(t, int64(1700000000), update.UpdatedAt)
}

func TestParseMessage_Ping(t *testing.T) {
	client := NewClient(paidCfg(), nil)
	raw, _ := json.Marshal(map[string]any{"type": "ping", "message": 1700000000})
	assert.Nil(t, client.parseMessage(raw))
}

func TestParseMessage_Welcome(t *testing.T) {
	client := NewClient(paidCfg(), nil)
	raw, _ := json.Marshal(map[string]string{"type": "welcome"})
	assert.Nil(t, client.parseMessage(raw))
}

func TestGracefulClose_SendsUnsubscribe(t *testing.T) {
	unsubReceived := make(chan bool, 1)

	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		happyHandshake(t, conn)

		// Read messages until we get an unsubscribe.
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var cmd actionCableCommand
			if json.Unmarshal(raw, &cmd) == nil && cmd.Command == "unsubscribe" {
				unsubReceived <- true
				return
			}
		}
	})

	client := NewClient(paidCfg(), []string{"bitcoin"})
	client.SetURL(wsURL(srv))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Connect(ctx)
	require.NoError(t, err)

	require.NoError(t, client.Close())

	select {
	case <-unsubReceived:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("unsubscribe not received")
	}
}

func TestReconnect_OnServerDisconnect(t *testing.T) {
	var connectionCount atomic.Int32

	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		n := connectionCount.Add(1)

		happyHandshake(t, conn)

		if n == 1 {
			// First connection: drop immediately after handshake.
			_ = conn.Close()
			return
		}

		// Second connection: send a bare price update then keep alive.
		sendJSON(conn, wsPayload{
			CoinID:    "bitcoin",
			Price:     46000,
			PricePct:  1.0,
			MarketCap: 860e9,
			Volume:    31e9,
			Timestamp: time.Now().Unix(),
		})

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	client := NewClient(paidCfg(), []string{"bitcoin"})
	client.SetURL(wsURL(srv))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	updates, err := client.Connect(ctx)
	require.NoError(t, err)

	// Should receive update from the second connection after reconnect.
	select {
	case update := <-updates:
		require.NotNil(t, update)
		assert.Equal(t, "bitcoin", update.CoinID)
		assert.Equal(t, 46000.0, update.Price)
	case <-time.After(10 * time.Second):
		t.Fatal("no update received after reconnect")
	}

	require.NoError(t, client.Close())

	assert.GreaterOrEqual(t, int(connectionCount.Load()), 2)
}

func TestContextCancelDuringBackoff(t *testing.T) {
	// Server that drops connection immediately after handshake.
	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		happyHandshake(t, conn)
		_ = conn.Close()
	})

	client := NewClient(paidCfg(), []string{"bitcoin"})
	client.SetURL(wsURL(srv))

	ctx, cancel := context.WithCancel(context.Background())

	updates, err := client.Connect(ctx)
	require.NoError(t, err)

	// Wait a moment for the read to fail and backoff to begin.
	time.Sleep(200 * time.Millisecond)

	// Cancel context — should exit promptly.
	start := time.Now()
	cancel()

	// Drain channel.
	for range updates {
	}

	elapsed := time.Since(start)
	assert.Less(t, elapsed, 3*time.Second, "should exit promptly on context cancel")
}

func TestCloseSuppressesReconnect(t *testing.T) {
	var connectionCount atomic.Int32

	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		connectionCount.Add(1)
		happyHandshake(t, conn)
		// Keep alive briefly then drop.
		time.Sleep(50 * time.Millisecond)
		_ = conn.Close()
	})

	client := NewClient(paidCfg(), []string{"bitcoin"})
	client.SetURL(wsURL(srv))

	ctx := context.Background()
	_, err := client.Connect(ctx)
	require.NoError(t, err)

	// Wait for server to drop, then close.
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, client.Close())

	// Give time for any reconnect that shouldn't happen.
	time.Sleep(2 * time.Second)

	// At most 1 additional reconnect may have started before Close() took effect.
	assert.LessOrEqual(t, int(connectionCount.Load()), 2,
		"should not keep reconnecting after Close()")
}

func TestNoGoroutineLeak(t *testing.T) {
	baseline := runtime.NumGoroutine()

	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		happyHandshake(t, conn)
		// Send a few pings then close.
		for i := 0; i < 3; i++ {
			sendJSON(conn, map[string]any{"type": "ping", "message": time.Now().Unix()})
			time.Sleep(50 * time.Millisecond)
		}
		_ = conn.Close()
	})

	client := NewClient(paidCfg(), []string{"bitcoin"})
	client.SetURL(wsURL(srv))

	ctx, cancel := context.WithCancel(context.Background())
	updates, err := client.Connect(ctx)
	require.NoError(t, err)

	// Cancel and drain.
	cancel()
	for range updates {
	}

	// Allow goroutines to clean up.
	time.Sleep(200 * time.Millisecond)

	after := runtime.NumGoroutine()
	// Allow some slack for runtime goroutines but ensure no major leak.
	assert.LessOrEqual(t, after, baseline+5, "goroutine leak detected: before=%d after=%d", baseline, after)
}

func TestConnect_APIKeyInQueryParam(t *testing.T) {
	var receivedKey string
	var mu sync.Mutex

	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		// Key is captured by the HTTP handler below; just do handshake.
		happyHandshake(t, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	// Override the handler to capture the query param.
	origHandler := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedKey = r.URL.Query().Get("x_cg_pro_api_key")
		mu.Unlock()
		origHandler.ServeHTTP(w, r)
	})

	cfg := &config.Config{APIKey: "my-secret-key", Tier: "paid"}
	client := NewClient(cfg, []string{"bitcoin"})
	client.SetURL(wsURL(srv))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Connect(ctx)
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, "my-secret-key", receivedKey)
	mu.Unlock()

	require.NoError(t, client.Close())
}

func TestConnect_SourceCLIQueryParam(t *testing.T) {
	var receivedSource string
	var mu sync.Mutex

	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		happyHandshake(t, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	origHandler := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedSource = r.URL.Query().Get("source")
		mu.Unlock()
		origHandler.ServeHTTP(w, r)
	})

	client := NewClient(paidCfg(), []string{"bitcoin"})
	client.SetURL(wsURL(srv))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Connect(ctx)
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, "cli", receivedSource)
	mu.Unlock()

	require.NoError(t, client.Close())
}

func TestConnect_UserAgentHeader(t *testing.T) {
	var gotUA string
	var mu sync.Mutex

	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		happyHandshake(t, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	// Wrap handler to capture User-Agent from the handshake request.
	origHandler := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotUA = r.Header.Get("User-Agent")
		mu.Unlock()
		origHandler.ServeHTTP(w, r)
	})

	client := NewClient(paidCfg(), []string{"bitcoin"})
	client.SetURL(wsURL(srv))
	client.UserAgent = "coingecko-cli/v1.2.3"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Connect(ctx)
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, "coingecko-cli/v1.2.3", gotUA)
	mu.Unlock()

	require.NoError(t, client.Close())
}

func TestCloseWithoutConnect(t *testing.T) {
	client := NewClient(paidCfg(), []string{"bitcoin"})
	// Close() should not hang if Connect() was never called.
	require.NoError(t, client.Close())
}

