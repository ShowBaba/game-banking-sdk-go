package gamebank_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	gamebank "github.com/ShowBaba/game-banking-sdk-go"
)

// lossyTransport forwards requests but reports the first POST /v1/transfers
// as a network failure after the engine has processed it.
type lossyTransport struct{ dropped bool }

func (l *lossyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := http.DefaultTransport.RoundTrip(r)
	if err == nil && !l.dropped && strings.HasSuffix(r.URL.Path, "/v1/transfers") {
		l.dropped = true
		res.Body.Close()
		return nil, errors.New("connection reset after the engine processed the request")
	}
	return res, err
}

// TestContract runs the PRD §16.2 journey against a real engine when GBE_BASE_URL is set.
func TestContract(t *testing.T) {
	base := os.Getenv("GBE_BASE_URL")
	if base == "" {
		t.Skip("set GBE_BASE_URL to run against an engine")
	}
	ctx := context.Background()
	uniq := func() string { return fmt.Sprintf("%x", time.Now().UnixNano()) }
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Sign up (a browser flow, so a plain request) for a developer session token.
	body, _ := json.Marshal(map[string]string{"name": "SDK Contract GO", "email": "sdk-go-" + uniq() + "@studio.test", "password": "contract-test-password"})
	res, err := http.Post(base+"/v1/auth/register", "application/json", bytes.NewReader(body))
	must(err)
	var reg struct {
		Session struct{ Token string } `json:"session"`
	}
	must(json.NewDecoder(res.Body).Decode(&reg))
	res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("register: %d", res.StatusCode)
	}

	mgmt, err := gamebank.NewManagement(gamebank.WithAPIKey(reg.Session.Token), gamebank.WithBaseURL(base))
	must(err)
	me, err := mgmt.Me(ctx)
	must(err)
	if me.Name != "SDK Contract GO" {
		t.Fatalf("me: %+v", me)
	}
	game, err := mgmt.Games.Create(ctx, "GO Game "+uniq(), "")
	must(err)
	all := []string{gamebank.ScopeBankRead, gamebank.ScopeBankWrite, gamebank.ScopeAccountRead, gamebank.ScopeAccountWrite,
		gamebank.ScopeTransferCreate, gamebank.ScopeFundingCreate, gamebank.ScopeTransactionReverse, gamebank.ScopeLimitsWrite}
	key, err := mgmt.APIKeys.Issue(ctx, game.ID, gamebank.IssueAPIKeyParams{Environment: "sandbox", Label: "contract", Scopes: all})
	must(err)
	if !strings.HasPrefix(key.Secret, "gbe_test_sk_") {
		t.Fatalf("secret %q", key.Secret)
	}
	bank, err := mgmt.Game(game.ID, "sandbox").Banks.Create(ctx, gamebank.CreateBankParams{Name: "Eko Trust Bank", BaseCurrency: "NGN"})
	must(err)
	if len(bank.SystemAccounts) != 8 { // incl. levies_payable
		t.Fatalf("system accounts: %d", len(bank.SystemAccounts))
	}
	readOnly, err := mgmt.APIKeys.Issue(ctx, game.ID, gamebank.IssueAPIKeyParams{Environment: "sandbox", Scopes: []string{gamebank.ScopeAccountRead}})
	must(err)

	var seen []gamebank.ResponseInfo
	gb, err := gamebank.NewClient(gamebank.WithAPIKey(key.Secret), gamebank.WithBaseURL(base),
		gamebank.WithResponseHook(func(r gamebank.ResponseInfo) { seen = append(seen, r) }))
	must(err)
	if gb.Environment != "sandbox" {
		t.Fatalf("environment %q", gb.Environment)
	}

	ada, err := gb.Players.Upsert(ctx, gamebank.UpsertPlayerParams{ExternalPlayerID: "ada-" + uniq(), DisplayName: "Ada Obi"})
	must(err)
	again, err := gb.Players.Upsert(ctx, gamebank.UpsertPlayerParams{ExternalPlayerID: ada.ExternalPlayerID})
	must(err)
	if again.ID != ada.ID {
		t.Fatal("upsert is not idempotent")
	}
	tunde, err := gb.Players.Upsert(ctx, gamebank.UpsertPlayerParams{ExternalPlayerID: "tunde-" + uniq(), DisplayName: "Tunde Bello"})
	must(err)
	adaAcct, err := gb.Accounts.Open(ctx, gamebank.OpenAccountParams{BankID: bank.ID, PlayerID: ada.ID})
	must(err)
	tundeAcct, err := gb.Accounts.Open(ctx, gamebank.OpenAccountParams{BankID: bank.ID, PlayerID: tunde.ID})
	must(err)

	ref := "pay-" + uniq()
	salary := gamebank.FundingParams{AccountID: adaAcct.ID, AmountMinor: 10_000_00, Reason: "salary", Reference: ref}
	funded, err := gb.Funding.Create(ctx, salary, gamebank.WithIdempotencyKey("fund-"+ref))
	must(err)
	if funded.Balance.PostedBalance != 10_000_00 {
		t.Fatalf("funded balance %d", funded.Balance.PostedBalance)
	}
	replay, err := gb.Funding.Create(ctx, salary, gamebank.WithIdempotencyKey("fund-"+ref))
	must(err)
	if replay.Transaction.ID != funded.Transaction.ID || !seen[len(seen)-1].Replayed {
		t.Fatal("replay did not return the original transaction")
	}
	changed := salary
	changed.AmountMinor = 1
	if _, err := gb.Funding.Create(ctx, changed, gamebank.WithIdempotencyKey("fund-"+ref)); !errors.Is(err, gamebank.ErrConflict) {
		t.Fatalf("key reuse: %v", err)
	}

	who, err := gb.NameEnquiry(ctx, bank.BankCode, tundeAcct.AccountNumber)
	must(err)
	if who.AccountName != "Tunde Bello" {
		t.Fatalf("name enquiry %+v", who)
	}
	tr, err := gb.Transfers.Create(ctx, gamebank.TransferParams{SourceAccountID: adaAcct.ID, DestinationAccountNumber: tundeAcct.AccountNumber, AmountMinor: 2_500_00, Narration: "Generator"})
	must(err)
	if tr.SourceBalance.PostedBalance != 7_500_00 || tr.DestinationBalance.PostedBalance != 2_500_00 {
		t.Fatalf("transfer balances %+v", tr)
	}

	_, err = gb.Transfers.Create(ctx, gamebank.TransferParams{SourceAccountID: tundeAcct.ID, DestinationAccountID: adaAcct.ID, AmountMinor: 99_999_00})
	if e, ok := gamebank.AsError(err); !errors.Is(err, gamebank.ErrInsufficientFunds) || !ok || e.Details["available_balance"] != float64(2_500_00) || e.RequestID == "" {
		t.Fatalf("insufficient funds: %v", err)
	}
	if _, err := gb.Accounts.Get(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, gamebank.ErrNotFound) {
		t.Fatalf("not found: %v", err)
	}
	ro, err := gamebank.NewClient(gamebank.WithAPIKey(readOnly.Secret), gamebank.WithBaseURL(base))
	must(err)
	_, err = ro.Transfers.Create(ctx, gamebank.TransferParams{SourceAccountID: adaAcct.ID, DestinationAccountID: tundeAcct.ID, AmountMinor: 1})
	if e, _ := gamebank.AsError(err); !errors.Is(err, gamebank.ErrPermissionDenied) || e.Code != "insufficient_scope" {
		t.Fatalf("scope: %v", err)
	}

	held, err := gb.Holds.Place(ctx, gamebank.PlaceHoldParams{AccountID: adaAcct.ID, AmountMinor: 3_000_00, Reason: "Order #1", DestinationAccountID: tundeAcct.ID})
	must(err)
	if held.Balance.PostedBalance != 7_500_00 || held.Balance.AvailableBalance != 4_500_00 || held.Hold.ReservedFeeMinor != 0 {
		t.Fatalf("hold balance %+v", held.Balance)
	}
	capt, err := gb.Holds.Capture(ctx, held.Hold.ID, gamebank.CaptureHoldParams{AmountMinor: 2_000_00})
	must(err)
	if capt.Hold.Status != "captured" || capt.SourceBalance.AvailableBalance != 5_500_00 {
		t.Fatalf("capture %+v", capt)
	}

	lt := &lossyTransport{}
	lossy, err := gamebank.NewClient(gamebank.WithAPIKey(key.Secret), gamebank.WithBaseURL(base), gamebank.WithHTTPClient(&http.Client{Transport: lt}))
	must(err)
	lost, err := lossy.Transfers.Create(ctx, gamebank.TransferParams{SourceAccountID: adaAcct.ID, DestinationAccountID: tundeAcct.ID, AmountMinor: 500_00})
	must(err)
	if !lt.dropped || lost.SourceBalance.PostedBalance != 5_000_00 {
		t.Fatalf("lost-response transfer: dropped=%v balance=%d", lt.dropped, lost.SourceBalance.PostedBalance)
	}
	bal, err := gb.Accounts.Balance(ctx, adaAcct.ID)
	must(err)
	if bal.PostedBalance != 5_000_00 {
		t.Fatalf("posted twice: %d", bal.PostedBalance)
	}

	var amounts []int64
	for item, err := range gb.Accounts.IterateTransactions(ctx, adaAcct.ID, 2) {
		must(err)
		amounts = append(amounts, item.AmountMinor)
	}
	if fmt.Sprint(amounts) != "[50000 200000 250000 1000000]" {
		t.Fatalf("history %v", amounts)
	}

	rev, err := gb.Transactions.Reverse(ctx, tr.Transaction.ID, gamebank.ReverseParams{Reason: "customer dispute"})
	must(err)
	orig, err := gb.Transactions.Get(ctx, tr.Transaction.ID)
	must(err)
	if rev.Type != "reversal" || orig.Status != "reversed" {
		t.Fatalf("reversal %s / %s", rev.Type, orig.Status)
	}

	// Fees: a ₦50 + 1% transfer fee (max ₦500) via the developer credential, a levy via the game key.
	i64 := func(v int64) *int64 { return &v }
	cfg := mgmt.Game(game.ID, "sandbox")
	fee, err := cfg.Fees.Create(ctx, bank.ID, gamebank.CreateFeeRuleParams{Code: "transfer_fee", Name: "Transfer fee",
		Event: gamebank.FeeEventTransfer, Model: gamebank.FeeModelCombined, FixedMinor: 50_00, PercentageBPS: 100, MaxFeeMinor: i64(500_00)})
	must(err)
	if fee.State != gamebank.FeeStateInEffect || fee.CreditAccount != "fee_income" {
		t.Fatalf("fee: %+v", fee)
	}
	levy, err := gb.Fees.Create(ctx, bank.ID, gamebank.CreateFeeRuleParams{Code: "stamp_duty", Name: "Stamp duty", Event: gamebank.FeeEventTransfer,
		Kind: gamebank.FeeKindLevy, Model: gamebank.FeeModelFixed, FixedMinor: 50_00, MinAmountMinor: i64(10_000_00)})
	must(err)
	if levy.CreditAccount != "levies_payable" {
		t.Fatalf("levy: %+v", levy)
	}
	if rules, err := gb.Fees.List(ctx, bank.ID); err != nil || len(rules) != 2 {
		t.Fatalf("list: %v %d", err, len(rules))
	}
	quote, err := gb.Fees.Quote(ctx, gamebank.QuoteFeesParams{BankID: bank.ID, AmountMinor: 1_000_00})
	must(err)
	if quote.FeesTotalMinor != 60_00 {
		t.Fatalf("quote: %+v", quote)
	}
	before, err := gb.Accounts.Balance(ctx, adaAcct.ID)
	must(err)
	pay := gamebank.TransferParams{SourceAccountID: adaAcct.ID, DestinationAccountID: tundeAcct.ID, AmountMinor: 1_000_00, MaxFeeMinor: i64(quote.FeesTotalMinor)}
	paid, err := gb.Transfers.Create(ctx, pay)
	must(err)
	if paid.FeesTotalMinor != 60_00 || paid.TotalDebitMinor != 1_060_00 || paid.SourceBalance.PostedBalance != before.PostedBalance-1_060_00 {
		t.Fatalf("paid: fees %d total %d balance %d", paid.FeesTotalMinor, paid.TotalDebitMinor, paid.SourceBalance.PostedBalance)
	}
	kinds := map[string]int{}
	for _, e := range paid.Transaction.Entries {
		kinds[e.Kind]++
	}
	if kinds["principal"] != 2 || kinds["fee"] != 2 {
		t.Fatalf("entry kinds %v", kinds)
	}
	latest, err := gb.Accounts.Transactions(ctx, adaAcct.ID, 2, "")
	must(err)
	labelled := false
	for _, l := range latest.Data {
		labelled = labelled || (l.Kind == "fee" && l.Narration != nil && *l.Narration == "Transfer fee")
	}
	if !labelled {
		t.Fatal("the fee should be its own labelled statement line")
	}
	_, err = cfg.Fees.Update(ctx, bank.ID, fee.ID, gamebank.UpdateFeeRuleParams{FixedMinor: i64(60_00)})
	must(err)
	if _, err := gb.Transfers.Create(ctx, pay); !errors.Is(err, gamebank.ErrFeeChanged) || !errors.Is(err, gamebank.ErrConflict) {
		t.Fatalf("fee_changed: %v", err)
	}
	books, err := gb.Banks.Books(ctx, bank.ID)
	must(err)
	if books.FeeIncomeMinor != 60_00 {
		t.Fatalf("books: %+v", books)
	}
	_, err = gb.Transactions.Reverse(ctx, paid.Transaction.ID, gamebank.ReverseParams{Reason: "refund"})
	must(err)
	if books, _ = gb.Banks.Books(ctx, bank.ID); books.FeeIncomeMinor != 0 {
		t.Fatalf("fee not refunded: %d", books.FeeIncomeMinor)
	}
	inactive := "inactive"
	if paused, err := cfg.Fees.Update(ctx, bank.ID, fee.ID, gamebank.UpdateFeeRuleParams{Status: &inactive}); err != nil || paused.State != gamebank.FeeStateInactive {
		t.Fatalf("pause: %v", err)
	}

	_, err = gb.Clock.Configure(ctx, gamebank.ConfigureClockParams{Mode: "manual"})
	must(err)
	adv, err := gb.Clock.Advance(ctx, gamebank.AdvanceClockParams{Days: 1})
	must(err)
	if adv.Clock.GameDay == "" {
		t.Fatal("no game day")
	}
	recon, err := gb.Reconcile(ctx)
	must(err)
	if !recon.OK {
		t.Fatalf("reconciliation: %v", recon.Differences)
	}

	// Renaming and deleting a game. Deleting revokes its keys at once.
	renamed, err := mgmt.Games.Update(ctx, game.ID, gamebank.UpdateGameParams{Name: "GO Game renamed"})
	must(err)
	if renamed.Name != "GO Game renamed" {
		t.Fatalf("rename: %+v", renamed)
	}
	must(mgmt.Games.Delete(ctx, game.ID))
	if _, err := mgmt.Games.Get(ctx, game.ID); !errors.Is(err, gamebank.ErrNotFound) {
		t.Fatalf("deleted game: %v", err)
	}
	if _, err := gb.Clock.Get(ctx); !errors.Is(err, gamebank.ErrAuthentication) {
		t.Fatalf("deleted game's key: %v", err)
	}
}
