package gamebank_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

// delivery is one webhook request a receiver got.
type delivery struct {
	header http.Header
	body   []byte
}

// receiver is a local webhook endpoint that keeps each raw delivery. The
// engine reaches it at GBE_WEBHOOK_RECEIVER_HOST (default host.docker.internal).
type receiver struct {
	url string
	got chan delivery
	srv *http.Server
	all []delivery
}

func startReceiver(t *testing.T) *receiver {
	t.Helper()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	host := os.Getenv("GBE_WEBHOOK_RECEIVER_HOST")
	if host == "" {
		host = "host.docker.internal"
	}
	r := &receiver{url: fmt.Sprintf("http://%s:%d/hooks", host, ln.Addr().(*net.TCPAddr).Port), got: make(chan delivery, 100)}
	r.srv = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.got <- delivery{header: req.Header.Clone(), body: body}
		w.WriteHeader(http.StatusNoContent)
	})}
	go r.srv.Serve(ln) //nolint:errcheck
	t.Cleanup(func() { r.srv.Close() })
	return r
}

// waitFor returns the first delivery (old or new) whose body contains s.
func (r *receiver) waitFor(t *testing.T, s string, fresh bool) delivery {
	t.Helper()
	if !fresh {
		for _, d := range r.all {
			if bytes.Contains(d.body, []byte(s)) {
				return d
			}
		}
	}
	deadline := time.After(20 * time.Second)
	for {
		select {
		case d := <-r.got:
			r.all = append(r.all, d)
			if bytes.Contains(d.body, []byte(s)) {
				return d
			}
		case <-deadline:
			t.Fatalf("no webhook delivery containing %q within 20s", s)
		}
	}
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
		gamebank.ScopeTransferCreate, gamebank.ScopeFundingCreate, gamebank.ScopeTransactionReverse, gamebank.ScopeLimitsWrite,
		gamebank.ScopeWebhooksRead, gamebank.ScopeWebhooksWrite,
		gamebank.ScopeMerchantsWrite, gamebank.ScopeCardsRead, gamebank.ScopeCardsWrite, gamebank.ScopeCardsCharge, gamebank.ScopeDepositsWrite}
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

	// Webhooks: a debit alert reaches a real endpoint, signed, and verifies with the SDK helper.
	if eps, err := mgmt.Game(game.ID, "sandbox").Webhooks.Endpoints.List(ctx); err != nil || len(eps) != 0 {
		t.Fatalf("developer webhooks: %v %d", err, len(eps))
	}
	if _, err := ro.Webhooks.Endpoints.List(ctx); !errors.Is(err, gamebank.ErrPermissionDenied) {
		t.Fatalf("webhooks scope: %v", err)
	}
	rc := startReceiver(t)
	ep, err := gb.Webhooks.Endpoints.Create(ctx, gamebank.CreateWebhookEndpointParams{URL: rc.url, EventTypes: []string{gamebank.EventAccountDebited}, Description: "contract"})
	must(err)
	if !strings.HasPrefix(ep.Secret, ep.SecretPrefix) || ep.SecretPrefix == "" {
		t.Fatalf("secret: %+v", ep)
	}
	if fetched, err := gb.Webhooks.Endpoints.Get(ctx, ep.ID); err != nil || fetched.Secret != "" {
		t.Fatalf("the secret is shown once: %v", err)
	}
	probe, err := gb.Webhooks.Endpoints.Test(ctx, ep.ID)
	must(err)
	if probe.State != "delivered" {
		t.Fatalf("test send: %+v (%v)", probe, probe.LastError)
	}
	sent, err := gb.Transfers.Create(ctx, gamebank.TransferParams{SourceAccountID: adaAcct.ID, DestinationAccountID: tundeAcct.ID, AmountMinor: 5_00, Narration: "Webhook check"})
	must(err)
	hit := rc.waitFor(t, sent.Transaction.ID, false)
	event, err := gamebank.VerifyWebhook(hit.body, hit.header.Get(gamebank.WebhookSignatureHeader), ep.Secret)
	must(err)
	alert, err := event.Alert()
	must(err)
	if event.Type != gamebank.EventAccountDebited || alert.AccountID != adaAcct.ID || alert.TotalMinor != 5_00 ||
		alert.Counterparty == nil || alert.Counterparty.AccountName != "Tunde Bello" ||
		alert.BalanceAfter.PostedMinor != sent.SourceBalance.PostedBalance || hit.header.Get("GBE-Event-Id") != event.ID {
		t.Fatalf("alert: %+v %+v", event, alert)
	}
	feed, err := gb.Webhooks.Events.List(ctx, gamebank.ListWebhookEventsParams{Type: gamebank.EventAccountDebited, AccountID: adaAcct.ID, Limit: 1})
	must(err)
	if len(feed.Data) != 1 || feed.Data[0].ID != event.ID {
		t.Fatalf("feed: %+v", feed)
	}
	if got, err := gb.Webhooks.Events.Get(ctx, event.ID); err != nil || got.ID != event.ID {
		t.Fatalf("get event: %v", err)
	}
	if ds, err := gb.Webhooks.Events.Deliveries(ctx, event.ID); err != nil || len(ds) != 1 || ds[0].State != "delivered" {
		t.Fatalf("deliveries: %v %+v", err, ds)
	}
	rotated, err := gb.Webhooks.Endpoints.RotateSecret(ctx, ep.ID)
	must(err)
	if rotated.Secret == "" || rotated.Secret == ep.Secret || rotated.PreviousSecretExpiresAt == nil {
		t.Fatalf("rotation: %+v", rotated)
	}
	_, err = gb.Webhooks.Events.Redeliver(ctx, event.ID, ep.ID)
	must(err)
	redelivered := rc.waitFor(t, sent.Transaction.ID, true)
	verifier := gamebank.WebhookVerifier{Secrets: []string{rotated.Secret}}
	if _, err := verifier.Verify(redelivered.body, redelivered.header.Get(gamebank.WebhookSignatureHeader)); err != nil {
		t.Fatalf("new secret: %v", err)
	}
	if _, err := gamebank.VerifyWebhook(redelivered.body, redelivered.header.Get(gamebank.WebhookSignatureHeader), ep.Secret); err != nil {
		t.Fatalf("old secret during the overlap: %v", err)
	}
	disabled := "disabled"
	if up, err := gb.Webhooks.Endpoints.Update(ctx, ep.ID, gamebank.UpdateWebhookEndpointParams{Status: &disabled}); err != nil || up.Status != "disabled" {
		t.Fatalf("disable: %v", err)
	}
	must(gb.Webhooks.Endpoints.Delete(ctx, ep.ID))
	if _, err := gb.Webhooks.Endpoints.Get(ctx, ep.ID); !errors.Is(err, gamebank.ErrNotFound) {
		t.Fatalf("deleted endpoint: %v", err)
	}

	// Merchants and cards: a shop with a merchant account, a card, a payment, a refund, a lost card.
	shop, err := gb.Merchants.Create(ctx, gamebank.CreateMerchantParams{ExternalMerchantID: "shop-" + uniq(), Name: "Mama Put", Category: "food"})
	must(err)
	if again, err := gb.Merchants.Create(ctx, gamebank.CreateMerchantParams{ExternalMerchantID: shop.ExternalMerchantID, Name: "x"}); err != nil || again.ID != shop.ID {
		t.Fatalf("merchants are idempotent: %v", err)
	}
	till, err := gb.Accounts.Open(ctx, gamebank.OpenAccountParams{BankID: bank.ID, MerchantID: shop.ID})
	must(err)
	if till.OwnerType != "merchant" || till.MerchantID == nil || *till.MerchantID != shop.ID {
		t.Fatalf("merchant account: %+v", till)
	}
	_, err = cfg.Banks.Update(ctx, bank.ID, gamebank.UpdateBankParams{FeatureFlags: map[string]bool{"cards": true}})
	must(err)
	progName := "Classic"
	program, err := cfg.CardPrograms.Create(ctx, bank.ID, gamebank.CardProgramParams{Name: &progName, DailyLimitMinor: i64(3_000_00)})
	must(err)
	if len(program.BIN) != 6 {
		t.Fatalf("program: %+v", program)
	}
	issued, err := gb.Cards.Issue(ctx, gamebank.IssueCardParams{AccountID: adaAcct.ID, NameOnCard: "Ada Obi"})
	must(err)
	pan := issued.CardNumber
	if len(pan) != 16 || issued.Card.MaskedNumber != pan[:6]+"******"+pan[12:] {
		t.Fatalf("card: %+v", issued.Card)
	}
	cardBal, err := gb.Accounts.Balance(ctx, adaAcct.ID)
	must(err)
	spendable := cardBal.AvailableBalance
	auth, err := gb.CardAuthorizations.Authorize(ctx, gamebank.AuthorizeCardParams{CardNumber: pan, ExpiryMonth: issued.Card.ExpiryMonth,
		ExpiryYear: issued.Card.ExpiryYear, CVV: issued.CVV, MerchantAccountNumber: till.AccountNumber, AmountMinor: 200_00, Hold: true})
	must(err)
	if auth.Authorization.Status != "approved" || auth.Balance.AvailableBalance != spendable-200_00 {
		t.Fatalf("authorize: %+v", auth.Authorization)
	}
	captured, err := gb.CardAuthorizations.Capture(ctx, auth.Authorization.ID, gamebank.CaptureCardParams{AmountMinor: 150_00})
	must(err)
	if captured.Transaction.Type != "card_payment" || captured.MerchantBalance.PostedBalance != 150_00 || captured.CardBalance.AvailableBalance != spendable-150_00 {
		t.Fatalf("capture: %+v", captured)
	}
	refunded, err := gb.CardAuthorizations.Refund(ctx, auth.Authorization.ID, gamebank.RefundCardParams{AmountMinor: 50_00, Reason: "cold"})
	must(err)
	if refunded.Transaction.Type != "card_refund" || refunded.Authorization.RefundedMinor != 50_00 {
		t.Fatalf("refund: %+v", refunded)
	}
	instant, err := gb.CardAuthorizations.Authorize(ctx, gamebank.AuthorizeCardParams{CardID: issued.Card.ID, MerchantAccountID: till.ID, AmountMinor: 20_00})
	must(err)
	if instant.Authorization.Status != "captured" || instant.Transaction == nil || instant.Transaction.Type != "card_payment" ||
		instant.MerchantBalance == nil || instant.MerchantBalance.PostedBalance != 120_00 {
		t.Fatalf("instant capture: %+v", instant)
	}
	pending, err := gb.CardAuthorizations.Authorize(ctx, gamebank.AuthorizeCardParams{CardID: issued.Card.ID, MerchantAccountID: till.ID, AmountMinor: 10_00, Hold: true})
	must(err)
	if lost, err := gb.Cards.ReportLost(ctx, issued.Card.ID, "stolen"); err != nil || lost.Status != gamebank.CardStolen {
		t.Fatalf("report lost: %v", err)
	}
	refused, err := gb.CardAuthorizations.Authorize(ctx, gamebank.AuthorizeCardParams{CardID: issued.Card.ID, MerchantAccountID: till.ID, AmountMinor: 10_00})
	must(err)
	if code, declined := refused.Authorization.Declined(); !declined || code != gamebank.DeclineCardStolen {
		t.Fatalf("decline: %+v", refused.Authorization)
	}
	if c, err := gb.CardAuthorizations.Capture(ctx, pending.Authorization.ID, gamebank.CaptureCardParams{}); err != nil || c.Authorization.Status != "captured" {
		t.Fatalf("capture after loss: %v", err)
	}
	if cards, err := gb.Cards.List(ctx, gamebank.ListCardsParams{AccountID: adaAcct.ID}); err != nil || len(cards) != 1 {
		t.Fatalf("no replacement card: %v %d", err, len(cards))
	}
	if auths, err := gb.CardAuthorizations.List(ctx, gamebank.ListCardAuthorizationsParams{CardID: issued.Card.ID}); err != nil || len(auths) != 4 {
		t.Fatalf("authorizations: %v %d", err, len(auths))
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

	// Savings: ₦100,000 at 36.5% earns ₦100 a game day; a fixed deposit can be broken early.
	_, err = cfg.Banks.Update(ctx, bank.ID, gamebank.UpdateBankParams{FeatureFlags: map[string]bool{"savings": true}})
	must(err)
	saver, err := cfg.DepositProducts.Create(ctx, bank.ID, gamebank.DepositProductParams{Code: "easy_saver", Name: "Easy Saver", Kind: "savings",
		RateTiers: []gamebank.DepositTier{{FromMinor: 0, RateBPS: 3650}}})
	must(err)
	forfeit, penalty := 10_000, int64(100_00)
	term, err := cfg.DepositProducts.Create(ctx, bank.ID, gamebank.DepositProductParams{Code: "term", Name: "Term Deposit", Kind: "fixed_deposit",
		Terms: []gamebank.DepositTerm{{Days: 30, RateBPS: 3650}}, EarlyForfeitBPS: &forfeit, EarlyPenaltyFixedMinor: &penalty})
	must(err)
	savings, err := gb.Accounts.Open(ctx, gamebank.OpenAccountParams{BankID: bank.ID, PlayerID: ada.ID, ProductID: saver.ID})
	must(err)
	if savings.AccountType != "savings" {
		t.Fatalf("savings account: %+v", savings)
	}
	_, err = gb.Funding.Create(ctx, gamebank.FundingParams{AccountID: savings.ID, AmountMinor: 100_000_00, Reason: "savings", Reference: "save-" + uniq()})
	must(err)
	_, err = gb.Funding.Create(ctx, gamebank.FundingParams{AccountID: adaAcct.ID, AmountMinor: 100_000_00, Reason: "deposit", Reference: "fd-" + uniq()})
	must(err)
	fd, err := gb.FixedDeposits.Open(ctx, gamebank.OpenFixedDepositParams{SourceAccountID: adaAcct.ID, ProductID: term.ID, TermDays: 30, AmountMinor: 100_000_00})
	must(err)
	if q, err := gb.DepositProducts.Quote(ctx, gamebank.DepositQuoteParams{ProductID: term.ID, AmountMinor: 100_000_00, TermDays: 30}); err != nil || q.GrossInterestMinor != 3_000_00 {
		t.Fatalf("quote: %v %+v", err, q)
	}
	_, err = gb.Clock.Advance(ctx, gamebank.AdvanceClockParams{Days: 2})
	must(err)
	if in, err := gb.Accounts.Interest(ctx, savings.ID); err != nil || in.AccruedInterestMinor != 200_00 {
		t.Fatalf("interest: %v %+v", err, in)
	}
	bq, err := gb.FixedDeposits.BreakQuote(ctx, fd.FixedDeposit.ID)
	must(err)
	if bq.PayoutMinor != 100_000_00-100_00 {
		t.Fatalf("break quote: %+v", bq)
	}
	low := int64(50_00)
	if _, err := gb.FixedDeposits.Break(ctx, fd.FixedDeposit.ID, &low); !errors.Is(err, gamebank.ErrPenaltyChanged) {
		t.Fatalf("penalty cap: %v", err)
	}
	if br, err := gb.FixedDeposits.Break(ctx, fd.FixedDeposit.ID, &bq.PenaltyMinor); err != nil || br.FixedDeposit.Status != "broken" {
		t.Fatalf("break: %v", err)
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
