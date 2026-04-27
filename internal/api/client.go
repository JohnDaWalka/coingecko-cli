package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coingecko/coingecko-cli/internal/config"
)

const (
	maxErrorBodySize    = 1 << 20 // 1MB
	maxResponseBodySize = 50 << 20 // 50MB — guards against pathological upstream responses
)

var (
	ErrInvalidAPIKey  = fmt.Errorf("invalid API key — check your key with `cg status` or set a new one with `cg auth`")
	ErrPlanRestricted = fmt.Errorf("this endpoint requires a paid plan — upgrade at https://www.coingecko.com/en/api/pricing")
	ErrRateLimited    = fmt.Errorf("rate limited — please wait and try again")
)

// RateLimitError carries rate-limit metadata from a 429 response.
type RateLimitError struct {
	RetryAfter int       // seconds from Retry-After header; 0 if absent
	ResetAt    time.Time // from x-ratelimit-reset header; zero if absent
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited — retry after %d seconds", e.RetryAfter)
	}
	if !e.ResetAt.IsZero() {
		return fmt.Sprintf("rate limited — resets at %s", e.ResetAt.UTC().Format("15:04:05 UTC"))
	}
	return "rate limited — please wait and try again"
}

func (e *RateLimitError) Is(target error) bool {
	return target == ErrRateLimited
}

// apiErrorResponse covers CoinGecko's error JSON formats.
// The API may return:
//
//	{"status": {"error_code": N, "error_message": "..."}}
//	{"error": {"status": {"error_code": N, "error_message": "..."}}}
//	{"error": "some string"}
type apiErrorResponse struct {
	Status *struct {
		ErrorCode    int    `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"status"`
	Error json.RawMessage `json:"error"`
}

// extractMessage returns the best error message from the response body.
func (e *apiErrorResponse) extractMessage() string {
	if e.Status != nil && e.Status.ErrorMessage != "" {
		return e.Status.ErrorMessage
	}
	if len(e.Error) > 0 {
		// Try nested {"error": {"status": {"error_message": "..."}}}
		var nested struct {
			Status *struct {
				ErrorMessage string `json:"error_message"`
			} `json:"status"`
		}
		if json.Unmarshal(e.Error, &nested) == nil && nested.Status != nil && nested.Status.ErrorMessage != "" {
			return nested.Status.ErrorMessage
		}
		// Try simple {"error": "string"}
		var s string
		if json.Unmarshal(e.Error, &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

// classify401 determines whether a 401 response is an auth failure, a plan
// restriction, or unknown. CoinGecko returns 401 for both invalid API keys
// and plan/entitlement restrictions.
//
// Strategy: positive identification of auth failures and known plan cases.
// Unknown 401s are left unclassified rather than forced into either bucket.
func classify401(apiErr apiErrorResponse, msg string) error {
	code := innerErrorCode(apiErr)
	lower := strings.ToLower(msg)

	// Check plan/entitlement restrictions first — these take priority over
	// the error code since CoinGecko sends plan restrictions with code 401.
	if isPlanRestriction(code, lower) {
		if msg != "" {
			return fmt.Errorf("%s (%w)", msg, ErrPlanRestricted)
		}
		return ErrPlanRestricted
	}

	// Known auth failures (error_code 401 or auth-related messages).
	if isAuthFailure(code, lower) {
		if msg != "" {
			return fmt.Errorf("%s (%w)", msg, ErrInvalidAPIKey)
		}
		return ErrInvalidAPIKey
	}

	// Unknown 401 — return a generic API error preserving the message.
	if msg != "" {
		return fmt.Errorf("API error 401: %s", msg)
	}
	return ErrInvalidAPIKey // bare 401 with no body → assume auth
}

func isAuthFailure(_ int, lowerMsg string) bool {
	for _, phrase := range []string{
		"invalid api key",
		"api key missing",
		"invalid demo api key",
		"invalid pro api key",
	} {
		if strings.Contains(lowerMsg, phrase) {
			return true
		}
	}
	return false
}

func isPlanRestriction(code int, lowerMsg string) bool {
	// Non-401 inner error codes are application-level restrictions.
	if code != 0 && code != 401 {
		return true
	}
	for _, phrase := range []string{
		"upgrade to a paid plan",
		"paid plan subscribers",
		"exclusive to paid plan",
	} {
		if strings.Contains(lowerMsg, phrase) {
			return true
		}
	}
	return false
}

// innerErrorCode extracts the application-level error code from the API response,
// checking both top-level and nested error formats.
func innerErrorCode(apiErr apiErrorResponse) int {
	if apiErr.Status != nil && apiErr.Status.ErrorCode != 0 {
		return apiErr.Status.ErrorCode
	}
	if len(apiErr.Error) > 0 {
		var nested struct {
			Status *struct {
				ErrorCode int `json:"error_code"`
			} `json:"status"`
		}
		if json.Unmarshal(apiErr.Error, &nested) == nil && nested.Status != nil {
			return nested.Status.ErrorCode
		}
	}
	return 0
}

type Client struct {
	http       *http.Client
	baseURLVal string // override; empty = use cfg.BaseURL()
	cfg        *config.Config
	UserAgent  string // sent with every request; set by cmd layer
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		http: &http.Client{Timeout: 30 * time.Second},
		cfg:  cfg,
	}
}

func NewClientWithHTTP(cfg *config.Config, httpClient *http.Client) *Client {
	return &Client{
		http: httpClient,
		cfg:  cfg,
	}
}

func (c *Client) SetBaseURL(url string) {
	c.baseURLVal = url
}

func (c *Client) baseURL() string {
	if c.baseURLVal != "" {
		return c.baseURLVal
	}
	return c.cfg.BaseURL()
}

func (c *Client) get(ctx context.Context, path string, result any) error {
	url := c.baseURL() + path
	if strings.Contains(path, "?") {
		url += "&source=cli"
	} else {
		url += "?source=cli"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	c.cfg.ApplyAuth(req)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return c.handleError(resp)
	}

	lr := io.LimitReader(resp.Body, maxResponseBodySize+1)
	dec := json.NewDecoder(lr)
	if err := dec.Decode(result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	return nil
}

func (c *Client) handleError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))

	// Parse CoinGecko error body for better classification.
	var apiErr apiErrorResponse
	_ = json.Unmarshal(body, &apiErr)
	msg := apiErr.extractMessage()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return classify401(apiErr, msg)

	case http.StatusForbidden:
		if msg != "" {
			return fmt.Errorf("%s (%w)", msg, ErrPlanRestricted)
		}
		return ErrPlanRestricted

	case http.StatusTooManyRequests:
		rle := &RateLimitError{}
		if retry := resp.Header.Get("Retry-After"); retry != "" {
			if secs, err := strconv.Atoi(retry); err == nil && secs > 0 {
				rle.RetryAfter = secs
			}
		}
		if reset := resp.Header.Get("x-ratelimit-reset"); reset != "" {
			// CoinGecko sends: "2026-03-09 03:28:00 +0000"
			if t, err := time.Parse("2006-01-02 15:04:05 -0700", reset); err == nil {
				rle.ResetAt = t
			}
		}
		return rle

	default:
		if msg != "" {
			return fmt.Errorf("API error %d: %s", resp.StatusCode, msg)
		}
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
}

func (c *Client) requirePaid() error {
	if !c.cfg.IsPaid() {
		return ErrPlanRestricted
	}
	return nil
}
