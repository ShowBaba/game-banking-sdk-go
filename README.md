# gamebank (Go)

Server-side SDK for the Game Banking Engine. Go 1.23+, standard library only.

> Game API keys move money. Use this SDK on your game servers only.

## Install

```bash
go get github.com/ShowBaba/game-banking-sdk-go
```

## Quick start

```go
import gamebank "github.com/ShowBaba/game-banking-sdk-go"

gb, err := gamebank.NewClient(
	gamebank.WithAPIKey(os.Getenv("GBE_API_KEY")),
	gamebank.WithBaseURL("https://your-engine.example.com"),
)

player, err := gb.Players.Upsert(ctx, gamebank.UpsertPlayerParams{ExternalPlayerID: "user-42", DisplayName: "Ada Obi"})
account, err := gb.Accounts.Open(ctx, gamebank.OpenAccountParams{BankID: bankID, PlayerID: player.ID})
_, err = gb.Funding.Create(ctx,
	gamebank.FundingParams{AccountID: account.ID, AmountMinor: 500_000, Reason: "salary", Reference: "payroll-2040-06-01-user-42"},
	gamebank.WithIdempotencyKey("payroll-2040-06-01-user-42"), // your stable ID: retrying never pays twice
)

_, err = gb.Transfers.Create(ctx, gamebank.TransferParams{SourceAccountID: account.ID, DestinationAccountNumber: "5915945174", AmountMinor: 1_000})
if errors.Is(err, gamebank.ErrInsufficientFunds) {
	e, _ := gamebank.AsError(err)
	log.Printf("available %v (request %s)", e.Details["available_balance"], e.RequestID)
}

for line, err := range gb.Accounts.IterateTransactions(ctx, account.ID, 50) {
	if err != nil {
		return err
	}
	fmt.Println(line.Direction, line.AmountDisplay)
}
```

Every call takes a `context.Context`. Errors are a single `*gamebank.Error`; test categories with `errors.Is(err, gamebank.ErrNotFound)` and friends, and read fields with `gamebank.AsError`.

## Fees and charges

```go
// Show the player what the transfer costs, then cap the fee at what they saw.
q, err := gb.Fees.Quote(ctx, gamebank.QuoteFeesParams{BankID: bankID, AmountMinor: 2_000_00})
quoted := q.FeesTotalMinor
r, err := gb.Transfers.Create(ctx, gamebank.TransferParams{SourceAccountID: account.ID,
	DestinationAccountNumber: "5915945174", AmountMinor: 2_000_00, MaxFeeMinor: &quoted})
if errors.Is(err, gamebank.ErrFeeChanged) {
	// the schedule changed: quote again
}
fmt.Println(r.Fees, r.TotalDebitMinor) // each fee is its own ledger and statement line

// Configure fees (bank:write): ₦50 + 1%, capped at ₦500, on transfers of ₦1,000 or more.
feeCap, fromAmount := int64(500_00), int64(1_000_00)
_, err = gb.Fees.Create(ctx, bankID, gamebank.CreateFeeRuleParams{Code: "transfer_fee", Name: "Transfer fee",
	Event: gamebank.FeeEventTransfer, Model: gamebank.FeeModelCombined, FixedMinor: 50_00, PercentageBPS: 100,
	MaxFeeMinor: &feeCap, MinAmountMinor: &fromAmount})
books, err := gb.Banks.Books(ctx, bankID) // FeeIncomeMinor, LeviesPayableMinor
```

## Merchants and cards

```go
// A shop and its merchant account (card payments settle here).
shop, err := gb.Merchants.Create(ctx, gamebank.CreateMerchantParams{ExternalMerchantID: "shop:mama-put", Name: "Mama Put", Category: "food"})
till, err := gb.Accounts.Open(ctx, gamebank.OpenAccountParams{BankID: bankID, MerchantID: shop.ID})

// Once per bank: turn on cards and add a program (bank:write).
_, err = gb.Banks.Update(ctx, bankID, gamebank.UpdateBankParams{FeatureFlags: map[string]bool{"cards": true}})
name := "Classic"
_, err = gb.CardPrograms.Create(ctx, bankID, gamebank.CardProgramParams{Name: &name})

// Issue a card: CardNumber and CVV are shown only now.
issued, err := gb.Cards.Issue(ctx, gamebank.IssueCardParams{AccountID: accountID})

// Pay: an approved payment is charged at once (status "captured"). A decline is a result, not an error.
paid, err := gb.CardAuthorizations.Authorize(ctx, gamebank.AuthorizeCardParams{CardID: issued.Card.ID, MerchantAccountID: till.ID, AmountMinor: 2_500_00})
if code, declined := paid.Authorization.Declined(); declined {
	showPlayer(code) // e.g. gamebank.DeclineInsufficientFunds
}

// Or hold first when the final amount or the sale isn't settled yet (a fuel pump, a pre-order).
held, err := gb.CardAuthorizations.Authorize(ctx, gamebank.AuthorizeCardParams{CardID: issued.Card.ID, MerchantAccountID: till.ID, AmountMinor: 2_500_00, Hold: true})
_, err = gb.CardAuthorizations.Capture(ctx, held.Authorization.ID, gamebank.CaptureCardParams{AmountMinor: 2_100_00}) // or Release; Refund later, in parts

_, err = gb.Cards.ReportLost(ctx, issued.Card.ID, "stolen") // final; no replacement is issued
```

## Savings and fixed deposits

```go
// Once per bank: turn savings on and define products (bank:write).
_, err = gb.Banks.Update(ctx, bankID, gamebank.UpdateBankParams{FeatureFlags: map[string]bool{"savings": true}})
saver, err := gb.DepositProducts.Create(ctx, bankID, gamebank.DepositProductParams{Code: "easy_saver", Name: "Easy Saver", Kind: "savings",
	RateTiers: []gamebank.DepositTier{{FromMinor: 0, RateBPS: 300}}})
term, err := gb.DepositProducts.Create(ctx, bankID, gamebank.DepositProductParams{Code: "term", Name: "Term Deposit", Kind: "fixed_deposit",
	Terms: []gamebank.DepositTerm{{Days: 90, RateBPS: 800}}})

// A savings account earns daily on the game clock and is paid monthly.
savings, err := gb.Accounts.Open(ctx, gamebank.OpenAccountParams{BankID: bankID, PlayerID: playerID, ProductID: saver.ID})
position, err := gb.Accounts.Interest(ctx, savings.ID) // AccruedInterestMinor, NextPayoutDay, Payouts…

// A fixed deposit (deposits:write); break it early with a quote first.
fd, err := gb.FixedDeposits.Open(ctx, gamebank.OpenFixedDepositParams{SourceAccountID: accountID, ProductID: term.ID, TermDays: 90, AmountMinor: 50_000_00})
quote, err := gb.FixedDeposits.BreakQuote(ctx, fd.FixedDeposit.ID)
_, err = gb.FixedDeposits.Break(ctx, fd.FixedDeposit.ID, &quote.PenaltyMinor) // errors.Is(err, gamebank.ErrPenaltyChanged) if it rose
```

## Webhooks: debit and credit alerts

Every money movement on a player's account sends a signed `account.debited` or `account.credited` event to your endpoint. Verify it with the **raw** body:

```go
// Register once (webhooks:write). The secret is shown only here.
ep, err := gb.Webhooks.Endpoints.Create(ctx, gamebank.CreateWebhookEndpointParams{
	URL: "https://game.example.com/gbe/webhooks", EventTypes: []string{gamebank.EventAccountDebited, gamebank.EventAccountCredited},
})

http.HandleFunc("POST /gbe/webhooks", func(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	event, err := gamebank.VerifyWebhook(body, r.Header.Get(gamebank.WebhookSignatureHeader), os.Getenv("GBE_WEBHOOK_SECRET"))
	if err != nil { // errors.Is(err, gamebank.ErrWebhookVerification)
		http.Error(w, "bad signature", http.StatusBadRequest)
		return
	}
	if event.Type == gamebank.EventAccountDebited {
		alert, _ := event.Alert() // AmountMinor, Charges, TotalMinor, BalanceAfter, Counterparty, Sequence…
		notifyPlayer(alert.PlayerID, "-"+alert.TotalDisplay)
	}
	w.WriteHeader(http.StatusNoContent) // answer within 10s; delivery is at least once, so dedupe on event.ID
})
```

`gb.Webhooks.Endpoints` also has `List`, `Get`, `Update` (Status `"disabled"` / `"active"`), `Delete`, `RotateSecret` (the old secret keeps signing for 24 hours; verify with both using `WebhookVerifier{Secrets: …}`) and `Test`. `gb.Webhooks.Events` has `List` / `Iterate` (the last 30 days, filterable by type and account, also a polling feed), `Get`, `Deliveries` and `Redeliver`.

## Setup and CI

```go
mgmt, _ := gamebank.NewManagement(gamebank.WithAPIKey(os.Getenv("GBE_DEV_CREDENTIAL")), gamebank.WithBaseURL(baseURL))
game, _ := mgmt.Games.Create(ctx, "Lagos Hustle", "")
key, _ := mgmt.APIKeys.Issue(ctx, game.ID, gamebank.IssueAPIKeyParams{Environment: "sandbox"})
fmt.Println(key.Secret) // shown once
_, _ = mgmt.Game(game.ID, "sandbox").Banks.Create(ctx, gamebank.CreateBankParams{Name: "Eko Trust Bank", BaseCurrency: "NGN"})
```

`mgmt.Games.Update(ctx, id, gamebank.UpdateGameParams{...})` renames or suspends a game; `mgmt.Games.Delete(ctx, id)` deletes it and revokes its keys.

## Behaviour

Retries, idempotency keys and money helpers follow the shared contract in [`CONTRACT.md`](CONTRACT.md). Options: `WithTimeout`, `WithMaxRetries`, `WithHTTPClient`, `WithResponseHook`; per call: `WithIdempotencyKey`, `WithRequestMaxRetries`. `ToMinor("100.50", 2)` and `FormatMinor(10050, 2)` convert amounts without floating point.

## Development

Unit tests need no network:

```bash
go test -race ./...
```

The contract test runs the full journey against a real engine. `ci/docker-compose.yml` starts one (PostgreSQL plus the engine's published image) on port 8080:

```bash
docker compose -f ci/docker-compose.yml up -d && ./ci/wait-for-engine.sh
GBE_BASE_URL=http://localhost:8080 go test -race ./...
docker compose -f ci/docker-compose.yml down -v
```

To test against a local engine build instead, build its image and set `GBE_ENGINE_IMAGE` to that tag. CI (`.github/workflows/ci.yml`) runs both on every push.
