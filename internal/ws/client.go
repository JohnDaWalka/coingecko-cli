package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coingecko/coingecko-cli/internal/api"
	"github.com/coingecko/coingecko-cli/internal/config"
	"github.com/gorilla/websocket"
)

const (
	// DefaultWSURL is the CoinGecko WebSocket streaming endpoint.
	DefaultWSURL = "wss://stream.coingecko.com/v1"
	// ChannelID is the ActionCable channel identifier for price streaming.
	ChannelID      = `{"channel":"CGSimplePrice"}`
	backoffMin     = 1 * time.Second
	backoffMax     = 30 * time.Second
	welcomeTimeout = 10 * time.Second
	confirmTimeout = 10 * time.Second
	// livenessTimeout is the maximum time to wait for any inbound message
	// (data or ping) before treating the connection as stale. CoinGecko sends
	// pings every ~3s and data every ~10s, so 60s provides ample headroom.
	livenessTimeout = 60 * time.Second
)

// CoinUpdate represents a parsed price update from the WebSocket stream.
type CoinUpdate struct {
	CoinID    string  `json:"coin_id"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change_24h_pct"`
	MarketCap float64 `json:"market_cap"`
	Volume24h float64 `json:"volume_24h"`
	UpdatedAt int64   `json:"updated_at"`
}

// Client manages a WebSocket connection to CoinGecko's streaming API.
type Client struct {
	cfg       *config.Config
	coinIDs   []string
	wsURL     string
	UserAgent string // sent with WebSocket handshake; set by cmd layer

	conn    *websocket.Conn
	updates chan *CoinUpdate
	done    chan struct{} // closed when readLoop exits for good
	started atomic.Bool  // true once readLoop goroutine is launched
	closing atomic.Bool  // set by Close(), suppresses reconnect
	mu      sync.Mutex   // protects conn
}

// NewClient creates a new WebSocket streaming client.
func NewClient(cfg *config.Config, coinIDs []string) *Client {
	return &Client{
		cfg:     cfg,
		coinIDs: coinIDs,
		wsURL:   DefaultWSURL,
		updates: make(chan *CoinUpdate, 64),
		done:    make(chan struct{}),
	}
}

// SetURL overrides the WebSocket URL (for testing).
func (c *Client) SetURL(url string) {
	c.wsURL = url
}

// Connect establishes the WebSocket connection and starts reading updates.
// Returns a channel that receives price updates. The channel is closed when
// the client is shut down or the context is canceled.
func (c *Client) Connect(ctx context.Context) (<-chan *CoinUpdate, error) {
	if !c.cfg.IsPaid() {
		return nil, api.ErrPlanRestricted
	}

	if err := c.connect(ctx); err != nil {
		return nil, fmt.Errorf("connecting to WebSocket: %w", err)
	}

	if err := c.subscribe(ctx); err != nil {
		c.closeConn()
		return nil, fmt.Errorf("subscribing: %w", err)
	}

	if err := c.setTokens(); err != nil {
		c.closeConn()
		return nil, fmt.Errorf("setting tokens: %w", err)
	}

	c.started.Store(true)
	go c.readLoop(ctx)

	return c.updates, nil
}

// Close gracefully shuts down the WebSocket connection. It is idempotent.
func (c *Client) Close() error {
	if !c.closing.CompareAndSwap(false, true) {
		// Already closing — wait for readLoop to finish if it was started.
		if c.started.Load() {
			<-c.done
		}
		return nil
	}

	c.mu.Lock()
	conn := c.conn
	if conn != nil {
		// Send unsubscribe before closing. Hold mu to prevent concurrent
		// writes from reconnect (gorilla/websocket doesn't support concurrent writers).
		unsub := actionCableCommand{
			Command:    "unsubscribe",
			Identifier: ChannelID,
		}
		data, _ := json.Marshal(unsub)
		_ = conn.WriteMessage(websocket.TextMessage, data)
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}
	c.mu.Unlock()

	if c.started.Load() {
		<-c.done
	}
	c.closeConn()
	return nil
}

// connect dials the WebSocket endpoint and waits for the welcome message.
func (c *Client) connect(ctx context.Context) error {
	url := c.wsURL + "?x_cg_pro_api_key=" + c.cfg.APIKey + "&source=cli"

	header := http.Header{}
	header.Set("User-Agent", c.UserAgent)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, url, header)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Wait for welcome message.
	if err := c.waitForType("welcome", welcomeTimeout); err != nil {
		c.closeConn()
		return fmt.Errorf("waiting for welcome: %w", err)
	}

	return nil
}

// subscribe sends the subscribe command and waits for confirmation.
func (c *Client) subscribe(ctx context.Context) error {
	sub := actionCableCommand{
		Command:    "subscribe",
		Identifier: ChannelID,
	}
	data, _ := json.Marshal(sub)

	c.mu.Lock()
	err := c.conn.WriteMessage(websocket.TextMessage, data)
	c.mu.Unlock()
	if err != nil {
		return err
	}

	// CoinGecko sends {"code":2000,"message":"Subscription is successful..."}
	// rather than ActionCable's standard {"type":"confirm_subscription"}.
	return c.waitForSubscription(confirmTimeout)
}

// setTokens tells the server which coin IDs to stream.
func (c *Client) setTokens() error {
	inner := map[string]any{
		"action":  "set_tokens",
		"coin_id": c.coinIDs,
	}
	innerData, _ := json.Marshal(inner)

	msg := actionCableCommand{
		Command:    "message",
		Identifier: ChannelID,
		Data:       string(innerData),
	}
	data, _ := json.Marshal(msg)

	c.mu.Lock()
	err := c.conn.WriteMessage(websocket.TextMessage, data)
	c.mu.Unlock()
	return err
}

// readLoop reads messages from the WebSocket and dispatches updates.
// On read failure, it reconnects unless Close() was called or ctx is done.
func (c *Client) readLoop(ctx context.Context) {
	defer close(c.done)
	defer close(c.updates)

	backoff := backoffMin

	for {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		// Set a liveness deadline so half-open connections are detected.
		// Refreshed on every successful read (data or ping).
		_ = conn.SetReadDeadline(time.Now().Add(livenessTimeout))

		_, raw, err := conn.ReadMessage()
		if err != nil {
			if c.closing.Load() || ctx.Err() != nil {
				return
			}

			// Connection lost or stale — attempt reconnect with backoff.
			c.closeConn()

			if !c.reconnect(ctx, &backoff) {
				return
			}
			continue
		}

		// Reset backoff on successful read.
		backoff = backoffMin

		update := c.parseMessage(raw)
		if update == nil {
			continue
		}

		select {
		case c.updates <- update:
		case <-ctx.Done():
			return
		}
	}
}

// reconnect attempts to re-establish the WebSocket connection with exponential backoff.
// Returns false if the client should stop (context done or closing).
func (c *Client) reconnect(ctx context.Context, backoff *time.Duration) bool {
	for {
		if c.closing.Load() || ctx.Err() != nil {
			return false
		}

		// Interruptible backoff with jitter.
		jitter := time.Duration(rand.Int64N(int64(*backoff / 4)))
		wait := *backoff + jitter

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return false
		}

		// Check again after waking — Close() may have been called during sleep.
		if c.closing.Load() {
			return false
		}

		// Increase backoff for next attempt.
		*backoff *= 2
		if *backoff > backoffMax {
			*backoff = backoffMax
		}

		if err := c.connect(ctx); err != nil {
			continue
		}
		if err := c.subscribe(ctx); err != nil {
			c.closeConn()
			continue
		}
		if err := c.setTokens(); err != nil {
			c.closeConn()
			continue
		}

		return true
	}
}

// closeConn closes the current WebSocket connection.
func (c *Client) closeConn() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// waitForType reads messages until a message of the given type is received or timeout expires.
func (c *Client) waitForType(msgType string, timeout time.Duration) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("reading %s: %w", msgType, err)
		}

		var msg actionCableMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Type == msgType {
			return nil
		}
	}
}

// parseMessage parses a raw WebSocket message into a CoinUpdate.
// Returns nil for non-data messages (ping, welcome, subscription confirmations).
//
// CoinGecko sends price updates as bare JSON payloads (not wrapped in an
// ActionCable envelope):
//
//	{"c":"C1","i":"bitcoin","p":69982.9,"pp":-0.11,"m":1.39e12,"v":4.68e10,"t":1773277871}
func (c *Client) parseMessage(raw []byte) *CoinUpdate {
	// Try bare payload first (most common message type).
	var data wsPayload
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}

	// Skip control messages: ping ({"type":"ping",...}), welcome, subscription confirms.
	// These parse into wsPayload with an empty CoinID.
	if data.CoinID == "" {
		return nil
	}

	return &CoinUpdate{
		CoinID:    data.CoinID,
		Price:     data.Price,
		Change24h: data.PricePct,
		MarketCap: data.MarketCap,
		Volume24h: data.Volume,
		UpdatedAt: data.Timestamp,
	}
}

// waitForSubscription waits for CoinGecko's subscription confirmation.
// CoinGecko sends {"code":2000,"message":"Subscription is successful..."} instead
// of the standard ActionCable {"type":"confirm_subscription"}.
func (c *Client) waitForSubscription(timeout time.Duration) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("reading subscription confirm: %w", err)
		}

		var msg subscriptionResponse
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		// CoinGecko uses code 2000 for successful subscription.
		if msg.Code == 2000 {
			return nil
		}
		// Also accept standard ActionCable confirm.
		var acMsg actionCableMessage
		if json.Unmarshal(raw, &acMsg) == nil && acMsg.Type == "confirm_subscription" {
			return nil
		}
	}
}

// Protocol types.

type actionCableCommand struct {
	Command    string `json:"command"`
	Identifier string `json:"identifier"`
	Data       string `json:"data,omitempty"`
}

type actionCableMessage struct {
	Type       string          `json:"type,omitempty"`
	Identifier string          `json:"identifier,omitempty"`
	Message    json.RawMessage `json:"message,omitempty"`
}

type subscriptionResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// wsPayload is the inner message format for CGSimplePrice updates.
type wsPayload struct {
	Category  string  `json:"c"`
	CoinID    string  `json:"i"`
	Price     float64 `json:"p"`
	PricePct  float64 `json:"pp"`
	MarketCap float64 `json:"m"`
	Volume    float64 `json:"v"`
	Timestamp int64   `json:"t"`
}
