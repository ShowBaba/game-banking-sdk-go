package gamebank

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// reply is a scripted response; err simulates a network failure.
type reply struct {
	status  int
	body    any
	headers map[string]string
	err     error
}

type fakeRT struct {
	replies []reply
	calls   []*http.Request
}

func (f *fakeRT) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls = append(f.calls, r)
	if len(f.replies) == 0 {
		return nil, errors.New("no more scripted replies")
	}
	rep := f.replies[0]
	f.replies = f.replies[1:]
	if rep.err != nil {
		return nil, rep.err
	}
	var text string
	switch b := rep.body.(type) {
	case string:
		text = b
	case nil:
	default:
		raw, _ := json.Marshal(b)
		text = string(raw)
	}
	h := http.Header{"Content-Type": {"application/json"}}
	for k, v := range rep.headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: rep.status, Header: h, Body: io.NopCloser(strings.NewReader(text)), Request: r}, nil
}

func fakeClient(t *testing.T, replies []reply, opts ...Option) (*Client, *fakeRT, *[]time.Duration) {
	t.Helper()
	rt := &fakeRT{replies: replies}
	opts = append([]Option{WithAPIKey("gbe_test_sk_abc"), WithBaseURL("http://engine.test/"), WithHTTPClient(&http.Client{Transport: rt})}, opts...)
	c, err := NewClient(opts...)
	if err != nil {
		t.Fatal(err)
	}
	waits := &[]time.Duration{}
	c.t.cfg.sleep = func(_ context.Context, d time.Duration) error { *waits = append(*waits, d); return nil }
	return c, rt, waits
}

func apiErr(status int, code string, details map[string]any) reply {
	return reply{status: status, body: map[string]any{"error": map[string]any{"code": code, "message": code, "details": details, "correlation_id": "req_1"}}}
}

var transferOK = reply{status: 201, body: map[string]any{"transaction": map[string]any{"id": "t1"}, "source_balance": map[string]any{}}}
var ctx = context.Background()

func TestConfiguration(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{{status: 200, body: map[string]any{"mode": "manual"}}})
	if c.Environment != "sandbox" {
		t.Fatalf("environment %q", c.Environment)
	}
	if _, err := c.Clock.Get(ctx); err != nil {
		t.Fatal(err)
	}
	if got := rt.calls[0].URL.String(); got != "http://engine.test/v1/clock" {
		t.Fatalf("url %s", got)
	}
	if rt.calls[0].Header.Get("Authorization") != "Bearer gbe_test_sk_abc" {
		t.Fatal("missing auth header")
	}
	t.Setenv("GBE_API_KEY", "")
	t.Setenv("GBE_BASE_URL", "")
	if _, err := NewClient(WithBaseURL("http://e")); err == nil {
		t.Fatal("missing key accepted")
	}
	if _, err := NewClient(WithAPIKey("gbe_test_sk_x")); err == nil {
		t.Fatal("missing base URL accepted")
	}
	t.Setenv("GBE_API_KEY", "gbe_live_sk_env")
	t.Setenv("GBE_BASE_URL", "http://env")
	if c, err := NewClient(); err != nil || c.Environment != "production" {
		t.Fatalf("env fallback: %v", err)
	}
}

func TestIdempotencyKeyReusedAcrossRetries(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{{err: errors.New("connection reset")}, {status: 503, body: map[string]any{}}, transferOK})
	r, err := c.Transfers.Create(ctx, TransferParams{SourceAccountID: "a", DestinationAccountID: "b", AmountMinor: 100})
	if err != nil || r.Transaction.ID != "t1" {
		t.Fatalf("transfer: %v %+v", err, r)
	}
	if len(rt.calls) != 3 {
		t.Fatalf("calls %d", len(rt.calls))
	}
	k := rt.calls[0].Header.Get("Idempotency-Key")
	if !strings.HasPrefix(k, "sdk_") {
		t.Fatalf("key %q", k)
	}
	for _, call := range rt.calls {
		if call.Header.Get("Idempotency-Key") != k {
			t.Fatal("retry used a different idempotency key")
		}
	}
}

func TestCallerKeyUsed(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{transferOK})
	if _, err := c.Transfers.Create(ctx, TransferParams{SourceAccountID: "a", DestinationAccountID: "b", AmountMinor: 1}, WithIdempotencyKey("order-42")); err != nil {
		t.Fatal(err)
	}
	if rt.calls[0].Header.Get("Idempotency-Key") != "order-42" {
		t.Fatal("caller key not used")
	}
}

func TestPlainPostNotRetried(t *testing.T) {
	for _, first := range []reply{{err: errors.New("reset")}, {status: 502, body: map[string]any{}}} {
		c, rt, _ := fakeClient(t, []reply{first, {status: 201, body: map[string]any{}}})
		if _, err := c.Accounts.Open(ctx, OpenAccountParams{BankID: "b", PlayerID: "p"}); err == nil {
			t.Fatal("expected an error")
		}
		if len(rt.calls) != 1 || rt.calls[0].Header.Get("Idempotency-Key") != "" {
			t.Fatalf("accounts.Open must not be retried (calls %d)", len(rt.calls))
		}
	}
}

func TestRateLimitHonoursRetryAfter(t *testing.T) {
	c, rt, waits := fakeClient(t, []reply{{status: 429, body: map[string]any{"error": map[string]any{"code": "rate_limited"}}, headers: map[string]string{"Retry-After": "3"}}, {status: 201, body: map[string]any{"id": "acc"}}})
	a, err := c.Accounts.Open(ctx, OpenAccountParams{BankID: "b", PlayerID: "p"})
	if err != nil || a.ID != "acc" || len(rt.calls) != 2 {
		t.Fatalf("open: %v calls %d", err, len(rt.calls))
	}
	if len(*waits) != 1 || (*waits)[0] != 3*time.Second {
		t.Fatalf("waits %v", *waits)
	}
}

func TestGetsAndUpsertsRetried(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{{err: errors.New("reset")}, {status: 200, body: map[string]any{"id": "p1"}}, {err: errors.New("reset")}, {status: 200, body: map[string]any{"id": "p2"}}})
	if p, err := c.Players.Get(ctx, "p1"); err != nil || p.ID != "p1" {
		t.Fatal(err)
	}
	if p, err := c.Players.Upsert(ctx, UpsertPlayerParams{ExternalPlayerID: "x"}); err != nil || p.ID != "p2" {
		t.Fatal(err)
	}
	if len(rt.calls) != 4 {
		t.Fatalf("calls %d", len(rt.calls))
	}
}

func TestStopsAfterMaxRetries(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{{status: 500, body: map[string]any{}}, {status: 500, body: map[string]any{}}}, WithMaxRetries(1))
	_, err := c.Accounts.Balance(ctx, "a")
	if !errors.Is(err, ErrServer) || len(rt.calls) != 2 {
		t.Fatalf("err %v calls %d", err, len(rt.calls))
	}
}

func TestRequestInProgressRetriedForKeyedRequests(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{apiErr(409, "request_in_progress", nil), transferOK})
	if _, err := c.Transfers.Create(ctx, TransferParams{SourceAccountID: "a", DestinationAccountID: "b", AmountMinor: 1}); err != nil || len(rt.calls) != 2 {
		t.Fatalf("err %v calls %d", err, len(rt.calls))
	}
}

func TestTypedErrors(t *testing.T) {
	c, _, _ := fakeClient(t, []reply{apiErr(422, "insufficient_funds", map[string]any{"available_balance": 5})})
	_, err := c.Transfers.Create(ctx, TransferParams{SourceAccountID: "a", DestinationAccountID: "b", AmountMinor: 9})
	e, ok := AsError(err)
	if !ok || !errors.Is(err, ErrInsufficientFunds) || !errors.Is(err, ErrUnprocessable) || errors.Is(err, ErrConflict) {
		t.Fatalf("categories wrong: %v", err)
	}
	if e.Details["available_balance"] != float64(5) || e.RequestID != "req_1" || e.Status != 422 {
		t.Fatalf("details: %+v", e)
	}
	c, _, _ = fakeClient(t, []reply{apiErr(409, "idempotency_key_reused", nil)})
	if _, err := c.Transfers.Create(ctx, TransferParams{SourceAccountID: "a", DestinationAccountID: "b", AmountMinor: 9}); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict: %v", err)
	}
}

func TestConnectionErrorAfterRetries(t *testing.T) {
	c, _, _ := fakeClient(t, []reply{{err: errors.New("refused")}, {err: errors.New("refused")}}, WithMaxRetries(1))
	_, err := c.Clock.Get(ctx)
	if !errors.Is(err, ErrConnection) {
		t.Fatalf("err %v", err)
	}
}

func TestCancelledContextIsNotRetried(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{{err: context.Canceled}, transferOK})
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.Clock.Get(cctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if len(rt.calls) > 1 {
		t.Fatalf("retried after cancellation: %d calls", len(rt.calls))
	}
}

func TestHTMLInsteadOfEngine(t *testing.T) {
	c, _, _ := fakeClient(t, []reply{{status: 200, body: "<!doctype html>", headers: map[string]string{"Content-Type": "text/html"}}})
	_, err := c.Clock.Get(ctx)
	e, ok := AsError(err)
	if !ok || e.Code != "invalid_response" || !strings.Contains(e.Message, "base URL") {
		t.Fatalf("err %v", err)
	}
}

func TestBadAmountRejectedBeforeSending(t *testing.T) {
	c, rt, _ := fakeClient(t, nil)
	_, err := c.Funding.Create(ctx, FundingParams{AccountID: "a", AmountMinor: -1, Reason: "r", Reference: "x"})
	if !errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrConnection) || len(rt.calls) != 0 {
		t.Fatalf("err %v calls %d", err, len(rt.calls))
	}
}

func TestMoney(t *testing.T) {
	for in, want := range map[string]int64{"100.50": 10050, "1,234.5": 123450, "0.07": 7, "-3.20": -320} {
		if got, err := ToMinor(in, 2); err != nil || got != want {
			t.Errorf("ToMinor(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if got, _ := ToMinor("42", 0); got != 42 {
		t.Error("whole units")
	}
	for _, bad := range []string{"1.005", "abc", "1.", "99999999999999999999"} {
		if _, err := ToMinor(bad, 2); err == nil {
			t.Errorf("ToMinor(%q) accepted", bad)
		}
	}
	for _, c := range []struct {
		minor int64
		exp   int
		want  string
	}{{10050, 2, "100.50"}, {7, 2, "0.07"}, {-5, 3, "-0.005"}, {42, 0, "42"}} {
		if got := FormatMinor(c.minor, c.exp); got != c.want {
			t.Errorf("FormatMinor(%d, %d) = %q; want %q", c.minor, c.exp, got, c.want)
		}
	}
}
