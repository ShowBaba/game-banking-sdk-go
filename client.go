// Package gamebank is the server-side Go SDK for the Game Banking Engine.
//
// Keep game API keys on your servers; never ship them in a game client.
//
//	gb, err := gamebank.NewClient(gamebank.WithAPIKey(os.Getenv("GBE_API_KEY")), gamebank.WithBaseURL("https://engine.example.com"))
//	res, err := gb.Transfers.Create(ctx, gamebank.TransferParams{SourceAccountID: a, DestinationAccountNumber: n, AmountMinor: 1000})
package gamebank

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"
)

func path(format string, ids ...string) string {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = url.PathEscape(id)
	}
	return fmt.Sprintf(format, args...)
}

func checkAmount(v int64, required bool) error {
	if v < 0 || (required && v == 0) {
		return &Error{Code: "validation_error", Message: "amount_minor must be a positive integer number of minor units"}
	}
	return nil
}

// Client is the runtime client for game servers, authenticated with a game API key.
type Client struct {
	t *transport
	// Environment is "sandbox" or "production", read from the API key's prefix.
	Environment  string
	Players      *PlayersService
	Accounts     *AccountsService
	Funding      *FundingService
	Transfers    *TransfersService
	Holds        *HoldsService
	Transactions *TransactionsService
	Banks        *BanksService
	Currencies   *CurrenciesService
	Clock        *ClockService
}

// NewClient builds a runtime client. Without options it reads GBE_API_KEY and GBE_BASE_URL.
func NewClient(opts ...Option) (*Client, error) {
	t, err := newTransport(opts)
	if err != nil {
		return nil, err
	}
	return newClient(t), nil
}

func newClient(t *transport) *Client {
	return &Client{
		t: t, Environment: t.environment,
		Players: &PlayersService{t}, Accounts: &AccountsService{t}, Funding: &FundingService{t},
		Transfers: &TransfersService{t}, Holds: &HoldsService{t}, Transactions: &TransactionsService{t},
		Banks: &BanksService{t}, Currencies: &CurrenciesService{t}, Clock: &ClockService{t},
	}
}

// NameEnquiry confirms a recipient before a transfer: it returns only the account name and status.
func (c *Client) NameEnquiry(ctx context.Context, bankCode, accountNumber string, opts ...RequestOption) (*NameEnquiry, error) {
	var out NameEnquiry
	q := url.Values{"bank_code": {bankCode}, "account_number": {accountNumber}}
	return &out, c.t.do(ctx, op{method: http.MethodGet, path: "/v1/name-enquiry", query: q}, &out, opts)
}

// Reconcile compares stored balances with the ledger for this game and environment.
func (c *Client) Reconcile(ctx context.Context, opts ...RequestOption) (*ReconciliationReport, error) {
	var out ReconciliationReport
	return &out, c.t.do(ctx, op{method: http.MethodGet, path: "/v1/reconciliation"}, &out, opts)
}

type PlayersService struct{ t *transport }

// Upsert maps your player ID to an engine player; repeat calls return the same player.
func (s *PlayersService) Upsert(ctx context.Context, p UpsertPlayerParams, opts ...RequestOption) (*Player, error) {
	var out Player
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/players", body: p, retrySafe: true}, &out, opts)
}

func (s *PlayersService) Get(ctx context.Context, playerID string, opts ...RequestOption) (*Player, error) {
	var out Player
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/players/%s", playerID)}, &out, opts)
}

// FindByExternalID looks a player up by your own player ID.
func (s *PlayersService) FindByExternalID(ctx context.Context, externalID string, opts ...RequestOption) (*Player, error) {
	var out Player
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/players", query: url.Values{"external_player_id": {externalID}}}, &out, opts)
}

type AccountsService struct{ t *transport }

func (s *AccountsService) Open(ctx context.Context, p OpenAccountParams, opts ...RequestOption) (*Account, error) {
	var out Account
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/accounts", body: p}, &out, opts)
}

func (s *AccountsService) Get(ctx context.Context, accountID string, opts ...RequestOption) (*Account, error) {
	var out Account
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/accounts/%s", accountID)}, &out, opts)
}

func (s *AccountsService) List(ctx context.Context, p ListAccountsParams, opts ...RequestOption) ([]Account, error) {
	q := url.Values{}
	if p.BankID != "" {
		q.Set("bank_id", p.BankID)
	}
	if p.PlayerID != "" {
		q.Set("player_id", p.PlayerID)
	}
	if p.IncludeSystem {
		q.Set("include_system", "true")
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	var out struct{ Data []Account }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/accounts", query: q}, &out, opts)
}

// UpdateStatus freezes (no debits), unfreezes ("active") or closes an account. Closing needs a zero balance.
func (s *AccountsService) UpdateStatus(ctx context.Context, accountID string, p UpdateAccountStatusParams, opts ...RequestOption) (*Account, error) {
	var out Account
	return &out, s.t.do(ctx, op{method: http.MethodPatch, path: path("/v1/accounts/%s", accountID), body: p}, &out, opts)
}

func (s *AccountsService) Balance(ctx context.Context, accountID string, opts ...RequestOption) (*Balance, error) {
	var out Balance
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/accounts/%s/balance", accountID)}, &out, opts)
}

// Transactions returns one page of history, newest first. Pass NextCursor back as cursor.
func (s *AccountsService) Transactions(ctx context.Context, accountID string, limit int, cursor string, opts ...RequestOption) (*HistoryPage, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var out HistoryPage
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/accounts/%s/transactions", accountID), query: q}, &out, opts)
}

// IterateTransactions yields every history line, newest first, fetching pages as needed:
//
//	for item, err := range gb.Accounts.IterateTransactions(ctx, id, 50) { ... }
func (s *AccountsService) IterateTransactions(ctx context.Context, accountID string, pageSize int, opts ...RequestOption) iter.Seq2[HistoryItem, error] {
	return func(yield func(HistoryItem, error) bool) {
		cursor := ""
		for {
			page, err := s.Transactions(ctx, accountID, pageSize, cursor, opts...)
			if err != nil {
				yield(HistoryItem{}, err)
				return
			}
			for _, item := range page.Data {
				if !yield(item, nil) {
					return
				}
			}
			if page.NextCursor == nil || *page.NextCursor == "" {
				return
			}
			cursor = *page.NextCursor
		}
	}
}

// Holds lists an account's holds; status may be "" for all.
func (s *AccountsService) Holds(ctx context.Context, accountID, status string, opts ...RequestOption) ([]Hold, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	var out struct{ Data []Hold }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/accounts/%s/holds", accountID), query: q}, &out, opts)
}

type FundingService struct{ t *transport }

// Create credits a player account from the bank's issuance account. Needs the funding:create scope.
func (s *FundingService) Create(ctx context.Context, p FundingParams, opts ...RequestOption) (*FundingResult, error) {
	if err := checkAmount(p.AmountMinor, true); err != nil {
		return nil, err
	}
	var out FundingResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/funding", body: p, idempotent: true}, &out, opts)
}

type TransfersService struct{ t *transport }

// Create moves money between two customer accounts in the same bank, atomically.
func (s *TransfersService) Create(ctx context.Context, p TransferParams, opts ...RequestOption) (*TransferResult, error) {
	if err := checkAmount(p.AmountMinor, true); err != nil {
		return nil, err
	}
	var out TransferResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/transfers", body: p, idempotent: true}, &out, opts)
}

type HoldsService struct{ t *transport }

// Place reserves funds: available balance drops, posted balance does not.
func (s *HoldsService) Place(ctx context.Context, p PlaceHoldParams, opts ...RequestOption) (*HoldResult, error) {
	if err := checkAmount(p.AmountMinor, true); err != nil {
		return nil, err
	}
	var out HoldResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/holds", body: p, idempotent: true}, &out, opts)
}

func (s *HoldsService) Get(ctx context.Context, holdID string, opts ...RequestOption) (*Hold, error) {
	var out Hold
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/holds/%s", holdID)}, &out, opts)
}

// Capture posts all (AmountMinor 0) or part of a hold as a transfer and releases the rest.
func (s *HoldsService) Capture(ctx context.Context, holdID string, p CaptureHoldParams, opts ...RequestOption) (*CaptureResult, error) {
	if err := checkAmount(p.AmountMinor, false); err != nil {
		return nil, err
	}
	var out CaptureResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/holds/%s/capture", holdID), body: p, idempotent: true}, &out, opts)
}

// Release cancels a hold and restores the available balance.
func (s *HoldsService) Release(ctx context.Context, holdID, reason string, opts ...RequestOption) (*HoldResult, error) {
	body := map[string]string{}
	if reason != "" {
		body["reason"] = reason
	}
	var out HoldResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/holds/%s/release", holdID), body: body, idempotent: true}, &out, opts)
}

type TransactionsService struct{ t *transport }

func (s *TransactionsService) Get(ctx context.Context, transactionID string, opts ...RequestOption) (*Transaction, error) {
	var out Transaction
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/transactions/%s", transactionID)}, &out, opts)
}

// Reverse posts a compensating transaction. Needs the transaction:reverse scope.
func (s *TransactionsService) Reverse(ctx context.Context, transactionID string, p ReverseParams, opts ...RequestOption) (*Transaction, error) {
	var out Transaction
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/transactions/%s/reverse", transactionID), body: p, idempotent: true}, &out, opts)
}

type BanksService struct{ t *transport }

// Create creates a bank with its own system ledger accounts.
func (s *BanksService) Create(ctx context.Context, p CreateBankParams, opts ...RequestOption) (*Bank, error) {
	var out Bank
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/banks", body: p}, &out, opts)
}

func (s *BanksService) List(ctx context.Context, opts ...RequestOption) ([]Bank, error) {
	var out struct{ Data []Bank }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/banks"}, &out, opts)
}

func (s *BanksService) Get(ctx context.Context, bankID string, opts ...RequestOption) (*Bank, error) {
	var out Bank
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/banks/%s", bankID)}, &out, opts)
}

func (s *BanksService) Update(ctx context.Context, bankID string, p UpdateBankParams, opts ...RequestOption) (*Bank, error) {
	var out Bank
	return &out, s.t.do(ctx, op{method: http.MethodPatch, path: path("/v1/banks/%s", bankID), body: p}, &out, opts)
}

func (s *BanksService) Limits(ctx context.Context, bankID string, opts ...RequestOption) (*Limits, error) {
	var out Limits
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/banks/%s/limits", bankID)}, &out, opts)
}

// SetLimits sets caps per scope (ScopePlayerFunding, …); a nil policy removes that scope's limits. Needs limits:write.
func (s *BanksService) SetLimits(ctx context.Context, bankID string, limits map[string]*LimitPolicy, opts ...RequestOption) (*Limits, error) {
	var out Limits
	return &out, s.t.do(ctx, op{method: http.MethodPut, path: path("/v1/banks/%s/limits", bankID), body: limits}, &out, opts)
}

type CurrenciesService struct{ t *transport }

// List returns standard ISO currencies plus this game's custom units.
func (s *CurrenciesService) List(ctx context.Context, opts ...RequestOption) ([]Currency, error) {
	var out struct{ Data []Currency }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/currencies"}, &out, opts)
}

// Create defines a custom virtual unit, e.g. {Code: "GOLD", Name: "Gold", Exponent: 0}.
func (s *CurrenciesService) Create(ctx context.Context, p CreateCurrencyParams, opts ...RequestOption) (*Currency, error) {
	var out Currency
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/currencies", body: p}, &out, opts)
}

type ClockService struct{ t *transport }

func (s *ClockService) Get(ctx context.Context, opts ...RequestOption) (*Clock, error) {
	var out Clock
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/clock"}, &out, opts)
}

// Configure changes mode, speed or time zone, or jumps forward. Needs limits:write.
func (s *ClockService) Configure(ctx context.Context, p ConfigureClockParams, opts ...RequestOption) (*Clock, error) {
	var out Clock
	return &out, s.t.do(ctx, op{method: http.MethodPut, path: "/v1/clock", body: p}, &out, opts)
}

// Advance moves a manual clock forward and runs the jobs that became due.
func (s *ClockService) Advance(ctx context.Context, p AdvanceClockParams, opts ...RequestOption) (*AdvanceClockResult, error) {
	var out AdvanceClockResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/clock/advance", body: p}, &out, opts)
}
