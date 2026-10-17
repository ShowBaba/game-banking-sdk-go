package gamebank

import (
	"context"
	"errors"
	"testing"
)

func TestSavingsRoutesAndPenaltyChanged(t *testing.T) {
	c, rt, _ := fakeClient(t, []reply{
		{status: 200, body: map[string]any{"account_id": "s1", "accrued_interest_minor": 5}},
		{status: 201, body: map[string]any{"fixed_deposit": map[string]any{"id": "f1"}}},
		apiErr(409, "penalty_changed", map[string]any{"quote": map[string]any{}}),
	})
	ctx := context.Background()
	in, err := c.Accounts.Interest(ctx, "s1")
	if err != nil || in.AccruedInterestMinor != 5 {
		t.Fatalf("interest: %v", err)
	}
	if _, err := c.FixedDeposits.Open(ctx, OpenFixedDepositParams{SourceAccountID: "a1", ProductID: "p1", TermDays: 30, AmountMinor: 1000}); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	_, err = c.FixedDeposits.Break(ctx, "f1", &one)
	if !errors.Is(err, ErrPenaltyChanged) || !errors.Is(err, ErrConflict) {
		t.Fatalf("penalty_changed: %v", err)
	}
	want := []string{"GET /v1/accounts/s1/interest", "POST /v1/fixed-deposits", "POST /v1/fixed-deposits/f1/break"}
	for i, r := range rt.calls {
		if got := r.Method + " " + r.URL.Path; got != want[i] {
			t.Errorf("call %d: %s", i, got)
		}
	}
	if rt.calls[1].Header.Get("Idempotency-Key") == "" {
		t.Fatal("open is a money call")
	}
}
