package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coingecko/coingecko-cli/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testClient(handler http.HandlerFunc) (*Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	cfg := &config.Config{APIKey: "test-key", Tier: config.TierDemo}
	c := NewClient(cfg)
	c.SetBaseURL(srv.URL)
	return c, srv
}

func TestUserAgentHeader(t *testing.T) {
	var gotUA string
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("{}"))
	})
	defer srv.Close()

	c.UserAgent = "coingecko-cli/v1.2.3"
	var result map[string]any
	_ = c.get(context.Background(), "/test", &result)
	assert.Equal(t, "coingecko-cli/v1.2.3", gotUA)
}

func TestAuthHeadersSent(t *testing.T) {
	var gotHeader string
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-cg-demo-api-key")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("{}"))
	})
	defer srv.Close()

	var result map[string]any
	_ = c.get(context.Background(), "/test", &result)
	assert.Equal(t, "test-key", gotHeader)
}

func TestProAuthHeaders(t *testing.T) {
	var gotHeader string
	cfg := &config.Config{APIKey: "pro-key", Tier: config.TierPaid}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-cg-pro-api-key")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	c := NewClient(cfg)
	c.SetBaseURL(srv.URL)
	var result map[string]any
	_ = c.get(context.Background(), "/test", &result)
	assert.Equal(t, "pro-key", gotHeader)
}

func TestError401InvalidKey(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"status":{"error_code":401,"error_message":"Invalid API key"}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAPIKey)
	assert.Contains(t, err.Error(), "Invalid API key")
}

func TestError401EmptyBody(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAPIKey)
}

func TestError401APIKeyMissing(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"status":{"error_code":401,"error_message":"API Key Missing"}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAPIKey)
}

func TestError401InvalidDemoKey(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"status":{"error_code":401,"error_message":"invalid demo api key usage"}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAPIKey)
}

func TestError401PlanUpgradeMessage(t *testing.T) {
	// 401 with plan-related message → ErrPlanRestricted
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"status":{"error_code":401,"error_message":"You need to upgrade to a paid plan"}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlanRestricted)
	assert.Contains(t, err.Error(), "upgrade to a paid plan")
}

func TestError401ExclusiveToPaidPlan(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"status":{"error_code":401,"error_message":"interval=daily is exclusive to paid plan subscribers"}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlanRestricted)
}

func TestError401NestedNon401Code(t *testing.T) {
	// 401 with non-401 inner error code (10012) → ErrPlanRestricted
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"status":{"timestamp":"2026-03-05T08:16:12.383+00:00","error_code":10012,"error_message":"Your request exceeds the allowed time range."}}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlanRestricted)
	assert.Contains(t, err.Error(), "allowed time range")
}

func TestError401InvalidProKey(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"status":{"error_code":401,"error_message":"invalid pro api key usage"}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAPIKey)
}

func TestError401PaidPlanSubscribersFeature(t *testing.T) {
	// Real backend message for paid-only features (e.g. show_max on /search/trending)
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"status":{"error_code":401,"error_message":"show_max is exclusive to paid plan subscribers"}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlanRestricted)
	assert.Contains(t, err.Error(), "exclusive to paid plan subscribers")
}

func TestError401UnknownMessage(t *testing.T) {
	// 401 with unrecognized message → generic API error, not forced into auth or plan
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"status":{"error_code":401,"error_message":"Something entirely unexpected"}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidAPIKey)
	assert.NotErrorIs(t, err, ErrPlanRestricted)
	assert.Contains(t, err.Error(), "API error 401")
	assert.Contains(t, err.Error(), "Something entirely unexpected")
}

func TestError403(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":"Your plan does not have access to this endpoint"}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlanRestricted)
	assert.Contains(t, err.Error(), "access")
}

func TestError403EmptyBody(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlanRestricted)
}

func TestError429(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry after 30 seconds")
}

func TestError429NoRetryAfter(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRateLimited)
}

func TestError429WithRateLimitReset(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ratelimit-reset", "2026-03-09 03:28:00 +0000")
		w.WriteHeader(429)
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRateLimited)

	var rle *RateLimitError
	require.ErrorAs(t, err, &rle)
	assert.Equal(t, 0, rle.RetryAfter) // no Retry-After header
	assert.False(t, rle.ResetAt.IsZero())
	assert.Equal(t, 2026, rle.ResetAt.Year())
	assert.Equal(t, time.Month(3), rle.ResetAt.Month())
	assert.Equal(t, 9, rle.ResetAt.Day())
	assert.Equal(t, 3, rle.ResetAt.Hour())
	assert.Equal(t, 28, rle.ResetAt.Minute())
	assert.Contains(t, err.Error(), "resets at 03:28:00 UTC")
}

func TestError429RetryAfterTakesPrecedence(t *testing.T) {
	// When both headers are present, RetryAfter should be set (used first by retry logic)
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "15")
		w.Header().Set("x-ratelimit-reset", "2026-03-09 03:28:00 +0000")
		w.WriteHeader(429)
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)

	var rle *RateLimitError
	require.ErrorAs(t, err, &rle)
	assert.Equal(t, 15, rle.RetryAfter)
	assert.False(t, rle.ResetAt.IsZero())
	assert.Contains(t, err.Error(), "retry after 15 seconds") // RetryAfter wins in message
}

func TestErrorUnknownStatusWithBody(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"status":{"error_code":500,"error_message":"Internal server error"}}`))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error 500")
	assert.Contains(t, err.Error(), "Internal server error")
}

func TestErrorUnknownStatusRawBody(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte("Bad Gateway"))
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error 502")
	assert.Contains(t, err.Error(), "Bad Gateway")
}

func TestSuccessResponseDecodes(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"bitcoin":{"usd":50000}}`))
	})
	defer srv.Close()

	var result PriceResponse
	err := c.get(context.Background(), "/test", &result)
	require.NoError(t, err)
	assert.Equal(t, float64(50000), result["bitcoin"]["usd"])
}

func TestRetryAfterInvalidFallback(t *testing.T) {
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "invalid-value")
		w.WriteHeader(429)
	})
	defer srv.Close()

	var result map[string]any
	err := c.get(context.Background(), "/test", &result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRateLimited)
}

func TestSourceCLIAppendedWithoutExistingQuery(t *testing.T) {
	var gotSource string
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		gotSource = r.URL.Query().Get("source")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("{}"))
	})
	defer srv.Close()

	var result map[string]any
	_ = c.get(context.Background(), "/test", &result)
	assert.Equal(t, "cli", gotSource)
}

func TestSourceCLIAppendedWithExistingQuery(t *testing.T) {
	var gotSource, gotExisting string
	c, srv := testClient(func(w http.ResponseWriter, r *http.Request) {
		gotSource = r.URL.Query().Get("source")
		gotExisting = r.URL.Query().Get("foo")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("{}"))
	})
	defer srv.Close()

	var result map[string]any
	_ = c.get(context.Background(), "/test?foo=bar", &result)
	assert.Equal(t, "cli", gotSource)
	assert.Equal(t, "bar", gotExisting)
}

func TestRequirePaid(t *testing.T) {
	cfg := &config.Config{Tier: config.TierDemo}
	c := NewClient(cfg)
	assert.ErrorIs(t, c.requirePaid(), ErrPlanRestricted)

	cfg2 := &config.Config{Tier: config.TierPaid}
	c2 := NewClient(cfg2)
	assert.NoError(t, c2.requirePaid())
}

