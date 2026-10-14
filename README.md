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
