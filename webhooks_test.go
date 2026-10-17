package gamebank

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// The engine's published test vector (docs/webhooks.md).
const (
	vectorBody = `{"hello":"world"}`
	vectorSig  = "f592bbf3951cfc94e560eecfb5d9dd4da6b0fff2e626235f8ab4b54860925d0b"
)

func at(unix int64) func() time.Time { return func() time.Time { return time.Unix(unix, 0) } }

func sign(secret string, ts int64, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%d.%s", ts, body)
	return hex.EncodeToString(m.Sum(nil))
}

func reason(t *testing.T, err error) string {
	t.Helper()
	var ve *WebhookVerificationError
	if !errors.As(err, &ve) || !errors.Is(err, ErrWebhookVerification) {
		t.Fatalf("want a WebhookVerificationError, got %v", err)
	}
	return ve.Reason
}

func TestVerifyWebhookVector(t *testing.T) {
	v := WebhookVerifier{Secrets: []string{"whsec_test"}, Now: at(1_700_000_010)}
	if _, err := v.Verify([]byte(vectorBody), "t=1700000000,v1="+vectorSig); err != nil {
		t.Fatalf("vector: %v", err)
	}
	// Any matching v1, and any of several secrets.
	h := "t=1700000000,v1=" + sign("whsec_new", 1_700_000_000, vectorBody) + ",v1=" + vectorSig
	for _, s := range []string{"whsec_new", "whsec_test"} {
		if _, err := (WebhookVerifier{Secrets: []string{s}, Now: at(1_700_000_010)}).Verify([]byte(vectorBody), h); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if _, err := (WebhookVerifier{Secrets: []string{"whsec_other", "whsec_test"}, Now: at(1_700_000_010)}).Verify([]byte(vectorBody), "t=1700000000,v1="+vectorSig); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyWebhookRejections(t *testing.T) {
	h := "t=1700000000,v1=" + vectorSig
	v := WebhookVerifier{Secrets: []string{"whsec_test"}, Now: at(1_700_000_010)}
	cases := []struct {
		v      WebhookVerifier
		body   string
		header string
		want   string
	}{
		{v, `{"hello":"world!"}`, h, "no_matching_signature"},
		{WebhookVerifier{Secrets: []string{"whsec_wrong"}, Now: at(1_700_000_010)}, vectorBody, h, "no_matching_signature"},
		{WebhookVerifier{Secrets: []string{"whsec_test"}, Now: at(1_700_000_301)}, vectorBody, h, "timestamp_out_of_tolerance"},
		{WebhookVerifier{Secrets: []string{"whsec_test"}, Now: at(1_699_999_000)}, vectorBody, h, "timestamp_out_of_tolerance"},
		{v, vectorBody, "v1=" + vectorSig, "malformed_header"},
		{v, vectorBody, "", "malformed_header"},
		{v, vectorBody, "t=1700000000,v1=zz", "malformed_header"},
		{WebhookVerifier{Secrets: []string{"s"}, Now: at(1_700_000_010)}, "plain text", "t=1700000000,v1=" + sign("s", 1_700_000_000, "plain text"), "invalid_payload"},
	}
	for i, c := range cases {
		_, err := c.v.Verify([]byte(c.body), c.header)
		if got := reason(t, err); got != c.want {
			t.Errorf("case %d: want %s, got %s", i, c.want, got)
		}
	}
	wide := WebhookVerifier{Secrets: []string{"whsec_test"}, Tolerance: 1000 * time.Second, Now: at(1_700_000_900)}
	if _, err := wide.Verify([]byte(vectorBody), h); err != nil {
		t.Fatalf("tolerance: %v", err)
	}
}

func TestWebhookEventAlert(t *testing.T) {
	body := `{"id":"ev1","type":"account.debited","created_at":"2026-10-16T09:00:00Z","game_time":null,"game_id":"g","environment":"sandbox",
		"data":{"account_id":"a1","total_minor":250,"charges":[{"kind":"fee","name":"Fee","fee_rule_id":null,"amount_minor":50}],
		"balance_after":{"posted_minor":10,"available_minor":5},"counterparty":{"type":"customer","account_name":"Bayo","account_number":"******4321","bank_code":"058"}}}`
	ts := time.Now().Unix()
	ev, err := VerifyWebhook([]byte(body), fmt.Sprintf("t=%d,v1=%s", ts, sign("s", ts, body)), "s")
	if err != nil {
		t.Fatal(err)
	}
	a, err := ev.Alert()
	if err != nil || a.AccountID != "a1" || a.TotalMinor != 250 || a.Charges[0].AmountMinor != 50 || a.BalanceAfter.AvailableMinor != 5 || a.Counterparty.AccountName != "Bayo" {
		t.Fatalf("alert: %v %+v", err, a)
	}
	if _, err := (&WebhookEvent{Type: EventWebhookTest}).Alert(); err == nil {
		t.Fatal("a test event is not an alert")
	}
}

func TestWebhookRoutes(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{
		{status: 201, body: map[string]any{"id": "e1", "secret": "whsec_x"}},
		{status: 200, body: map[string]any{"data": []any{map[string]any{"id": "ev1"}}, "next_cursor": "c2", "has_more": true}},
		{status: 200, body: map[string]any{"data": []any{map[string]any{"id": "ev2"}}, "next_cursor": nil, "has_more": false}},
		{status: 202, body: map[string]any{"data": []any{}}},
		{status: 204},
	})
	ctx := context.Background()
	ep, err := c.Webhooks.Endpoints.Create(ctx, CreateWebhookEndpointParams{URL: "https://hooks.example.com", EventTypes: []string{EventAccountDebited}})
	if err != nil || ep.Secret != "whsec_x" {
		t.Fatalf("create: %v", err)
	}
	var ids []string
	for ev, err := range c.Webhooks.Events.Iterate(ctx, ListWebhookEventsParams{Type: EventAccountDebited, AccountID: "a1"}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, ev.ID)
	}
	if _, err := c.Webhooks.Events.Redeliver(ctx, "ev1", "e1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Webhooks.Endpoints.Delete(ctx, "e1"); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[1] != "ev2" {
		t.Fatalf("ids: %v", ids)
	}
	want := []string{
		"POST /v1/webhooks/endpoints",
		"GET /v1/webhooks/events?account_id=a1&type=account.debited",
		"GET /v1/webhooks/events?account_id=a1&cursor=c2&type=account.debited",
		"POST /v1/webhooks/events/ev1/redeliver",
		"DELETE /v1/webhooks/endpoints/e1",
	}
	for i, r := range rt.calls {
		got := r.Method + " " + r.URL.Path
		if r.URL.RawQuery != "" {
			got += "?" + r.URL.RawQuery
		}
		if got != want[i] {
			t.Errorf("call %d: %s, want %s", i, got, want[i])
		}
	}
	var body map[string]any
	raw, _ := io.ReadAll(rt.calls[0].Body)
	_ = json.Unmarshal(raw, &body)
	if body["url"] != "https://hooks.example.com" || body["description"] != nil {
		t.Fatalf("create body: %s", raw)
	}
}
