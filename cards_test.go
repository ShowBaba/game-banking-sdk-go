package gamebank

import (
	"context"
	"encoding/json"
	"io"
	"testing"
)

func TestCardDeclineIsAResultAndRoutes(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{
		{status: 201, body: map[string]any{"authorization": map[string]any{"id": "a1", "status": "declined", "decline_code": "card_blocked"}}},
		{status: 200, body: map[string]any{"id": "c1", "status": "stolen"}},
		{status: 201, body: map[string]any{"refund": map[string]any{"id": "r1"}}},
		{status: 201, body: map[string]any{"id": "m1"}},
	})
	ctx := context.Background()
	r, err := c.CardAuthorizations.Authorize(ctx, AuthorizeCardParams{CardID: "c1", MerchantAccountID: "ma1", AmountMinor: 500, Hold: true})
	if err != nil {
		t.Fatal(err)
	}
	if code, declined := r.Authorization.Declined(); !declined || code != DeclineCardBlocked {
		t.Fatalf("declined: %v %s", declined, code)
	}
	if _, err := c.Cards.ReportLost(ctx, "c1", "stolen"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CardAuthorizations.Refund(ctx, "a1", RefundCardParams{AmountMinor: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Merchants.Create(ctx, CreateMerchantParams{ExternalMerchantID: "shop-1", Name: "Shop"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /v1/card-authorizations", "POST /v1/cards/c1/report-lost", "POST /v1/card-authorizations/a1/refunds", "POST /v1/merchants"}
	for i, r := range rt.calls {
		if got := r.Method + " " + r.URL.Path; got != want[i] {
			t.Errorf("call %d: %s, want %s", i, got, want[i])
		}
	}
	if rt.calls[0].Header.Get("Idempotency-Key") == "" || rt.calls[2].Header.Get("Idempotency-Key") == "" {
		t.Fatal("authorize and refund are money calls with idempotency keys")
	}
	var auth map[string]any
	raw0, _ := io.ReadAll(rt.calls[0].Body)
	_ = json.Unmarshal(raw0, &auth)
	if auth["capture"] != false || auth["card_id"] != "c1" {
		t.Fatalf("Hold must send capture false: %s", raw0)
	}
	var body map[string]any
	raw, _ := io.ReadAll(rt.calls[1].Body)
	_ = json.Unmarshal(raw, &body)
	if body["reason"] != "stolen" {
		t.Fatalf("report-lost body: %s", raw)
	}
	if _, err := c.CardAuthorizations.Authorize(ctx, AuthorizeCardParams{CardID: "c1", AmountMinor: 0}); err == nil {
		t.Fatal("a zero amount must be refused before sending")
	}
}
