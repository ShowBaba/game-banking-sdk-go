package gamebank

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK version.
const Version = "0.5.0"

// ResponseInfo is passed to the response hook after every HTTP response, including retried ones.
type ResponseInfo struct {
	Method    string
	Path      string
	Status    int
	RequestID string
	// Replayed is true when the engine returned a stored result for a repeated Idempotency-Key.
	Replayed bool
	// Attempt is 0 for the first attempt.
	Attempt int
}

// Option configures a client.
type Option func(*config)

type config struct {
	apiKey     string
	baseURL    string
	timeout    time.Duration
	maxRetries int
	httpClient *http.Client
	onResponse func(ResponseInfo)
	headers    map[string]string
	sleep      func(context.Context, time.Duration) error
}

// WithAPIKey sets the game API key or developer credential (default: GBE_API_KEY).
func WithAPIKey(key string) Option { return func(c *config) { c.apiKey = key } }

// WithBaseURL sets the engine's address (default: GBE_BASE_URL).
func WithBaseURL(u string) Option { return func(c *config) { c.baseURL = u } }

// WithTimeout sets the per-attempt timeout (default 30s).
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithMaxRetries sets how many extra attempts a retryable failure gets (default 2).
func WithMaxRetries(n int) Option { return func(c *config) { c.maxRetries = n } }

// WithHTTPClient sets the HTTP client, e.g. for proxies or tests.
func WithHTTPClient(hc *http.Client) Option { return func(c *config) { c.httpClient = hc } }

// WithResponseHook is called after every HTTP response.
func WithResponseHook(fn func(ResponseInfo)) Option { return func(c *config) { c.onResponse = fn } }

// RequestOption configures one call.
type RequestOption func(*callConfig)

type callConfig struct {
	idempotencyKey string
	maxRetries     *int
}

// WithIdempotencyKey sets your own stable key for a money-moving call.
// Retrying with the same key never moves money twice.
func WithIdempotencyKey(key string) RequestOption {
	return func(c *callConfig) { c.idempotencyKey = key }
}

// WithRequestMaxRetries overrides the client's retry count for one call.
func WithRequestMaxRetries(n int) RequestOption { return func(c *callConfig) { c.maxRetries = &n } }

type transport struct {
	cfg         config
	environment string
}

func newTransport(opts []Option) (*transport, error) {
	c := config{timeout: 30 * time.Second, maxRetries: 2, sleep: sleepCtx}
	for _, o := range opts {
		o(&c)
	}
	if c.apiKey == "" {
		c.apiKey = os.Getenv("GBE_API_KEY")
	}
	if c.baseURL == "" {
		c.baseURL = os.Getenv("GBE_BASE_URL")
	}
	c.apiKey = strings.TrimSpace(c.apiKey)
	if c.apiKey == "" {
		return nil, &Error{Code: "missing_api_key", Message: "pass WithAPIKey or set GBE_API_KEY"}
	}
	if c.baseURL == "" {
		return nil, &Error{Code: "missing_base_url", Message: "pass WithBaseURL (your engine's address) or set GBE_BASE_URL"}
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	if c.httpClient == nil {
		c.httpClient = &http.Client{}
	}
	return &transport{cfg: c, environment: EnvironmentOf(c.apiKey)}, nil
}

// EnvironmentOf reads the environment ("sandbox" or "production") a game API key belongs to from its prefix.
func EnvironmentOf(apiKey string) string {
	switch {
	case strings.HasPrefix(apiKey, "gbe_test_sk_"):
		return "sandbox"
	case strings.HasPrefix(apiKey, "gbe_live_sk_"):
		return "production"
	}
	return ""
}

func (t *transport) withHeaders(h map[string]string) *transport {
	c := t.cfg
	c.headers = map[string]string{}
	for k, v := range t.cfg.headers {
		c.headers[k] = v
	}
	for k, v := range h {
		c.headers[k] = v
	}
	return &transport{cfg: c, environment: t.environment}
}

// op describes one operation on the wire.
type op struct {
	method string
	path   string
	query  url.Values
	body   any
	// idempotent: money-moving; send an Idempotency-Key (generated when none is given).
	idempotent bool
	// retrySafe: safe to retry after a possible partial failure even without a key.
	retrySafe bool
}

func (t *transport) do(ctx context.Context, o op, out any, opts []RequestOption) error {
	var cc callConfig
	for _, f := range opts {
		f(&cc)
	}
	u := t.cfg.baseURL + o.path
	if len(o.query) > 0 {
		u += "?" + o.query.Encode()
	}
	key := cc.idempotencyKey
	if key == "" && o.idempotent {
		key = newKey()
	}
	var body []byte
	if o.body != nil {
		var err error
		if body, err = json.Marshal(o.body); err != nil {
			return fmt.Errorf("gamebank: encode request: %w", err)
		}
	}
	safe := o.method == http.MethodGet || o.method == http.MethodPut || key != "" || o.retrySafe
	retries := t.cfg.maxRetries
	if cc.maxRetries != nil {
		retries = *cc.maxRetries
	}

	for attempt := 0; ; attempt++ {
		err := t.once(ctx, o, u, key, body, out, attempt)
		if err == nil {
			return nil
		}
		var e *Error
		if !errors.As(err, &e) || attempt >= retries || !shouldRetry(e, safe) {
			return err
		}
		if err := t.cfg.sleep(ctx, RetryDelay(attempt, e.RetryAfter)); err != nil {
			return err
		}
	}
}

func (t *transport) once(ctx context.Context, o op, u, key string, body []byte, out any, attempt int) error {
	actx, cancel := context.WithTimeout(ctx, t.cfg.timeout)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(actx, o.method, u, rd)
	if err != nil {
		return fmt.Errorf("gamebank: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.cfg.apiKey)
	req.Header.Set("User-Agent", "gamebanking-sdk-go/"+Version)
	for k, v := range t.cfg.headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	res, err := t.cfg.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err() // the caller cancelled: do not retry
		}
		code, msg := "connection_error", fmt.Sprintf("could not reach the engine at %s: %v", t.cfg.baseURL, err)
		if actx.Err() != nil {
			code, msg = "timeout", fmt.Sprintf("no response from %s within %s", t.cfg.baseURL, t.cfg.timeout)
		}
		return &Error{Code: code, Message: msg, cause: err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &Error{Code: "connection_error", Message: "reading the response failed: " + err.Error(), cause: err}
	}
	rid := res.Header.Get("X-Request-Id")
	if t.cfg.onResponse != nil {
		t.cfg.onResponse(ResponseInfo{Method: o.method, Path: o.path, Status: res.StatusCode, RequestID: rid,
			Replayed: res.Header.Get("Idempotent-Replayed") == "true", Attempt: attempt})
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		if out == nil || res.StatusCode == http.StatusNoContent || len(raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return &Error{Status: res.StatusCode, Code: "invalid_response", RequestID: rid, cause: err,
				Message: fmt.Sprintf("expected JSON from the engine but got %s; check that the base URL (%s) points at the engine, not a website",
					contentType(res), t.cfg.baseURL)}
		}
		return nil
	}
	return toError(res, raw, rid)
}

func toError(res *http.Response, raw []byte, rid string) *Error {
	var env struct {
		Error struct {
			Code          string         `json:"code"`
			Message       string         `json:"message"`
			Details       map[string]any `json:"details"`
			CorrelationID string         `json:"correlation_id"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &env)
	e := &Error{Status: res.StatusCode, Code: env.Error.Code, Message: env.Error.Message, Details: env.Error.Details, RequestID: env.Error.CorrelationID}
	if e.Code == "" {
		e.Code = "http_" + strconv.Itoa(res.StatusCode)
	}
	if e.Message == "" {
		e.Message = http.StatusText(res.StatusCode)
	}
	if e.RequestID == "" {
		e.RequestID = rid
	}
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	if ra, err := strconv.ParseFloat(res.Header.Get("Retry-After"), 64); err == nil && ra > 0 {
		e.RetryAfter = ra
	}
	return e
}

func shouldRetry(e *Error, safe bool) bool {
	if e.Status == 429 {
		return true
	}
	if !safe {
		return false
	}
	switch e.Status {
	case 0:
		return e.Code == "connection_error" || e.Code == "timeout"
	case 500, 502, 503, 504:
		return true
	case 409:
		return e.Code == "request_in_progress"
	}
	return false
}

// RetryDelay is the wait before the next attempt: the server's Retry-After
// (capped at 60s) when it sent one, else exponential backoff with jitter.
func RetryDelay(attempt int, retryAfterSeconds float64) time.Duration {
	if retryAfterSeconds > 0 {
		return time.Duration(math.Min(retryAfterSeconds, 60) * float64(time.Second))
	}
	base := math.Min(0.5*math.Pow(2, float64(attempt)), 8)
	return time.Duration(base * (0.75 + mrand.Float64()*0.25) * float64(time.Second))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func newKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "sdk_" + hex.EncodeToString(b)
}

func contentType(res *http.Response) string {
	if ct := res.Header.Get("Content-Type"); ct != "" {
		return ct
	}
	return "an unknown content type"
}
