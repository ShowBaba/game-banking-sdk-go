package gamebank

import (
	"context"
	"net/http"
	"time"
)

// DepositTier is an annual rate from a balance upwards (300 = 3%).
type DepositTier struct {
	FromMinor int64 `json:"from_minor"`
	RateBPS   int   `json:"rate_bps"`
}

// DepositTerm is a fixed-deposit term on offer.
type DepositTerm struct {
	Days    int `json:"days"`
	RateBPS int `json:"rate_bps"`
}

// Maturity instructions.
const (
	MaturityPayout            = "payout"
	MaturityRolloverPrincipal = "rollover_principal"
	MaturityRolloverAll       = "rollover_all"
)

type DepositProduct struct {
	ID                         string        `json:"id"`
	BankID                     string        `json:"bank_id"`
	Code                       string        `json:"code"`
	Name                       string        `json:"name"`
	Kind                       string        `json:"kind"` // savings or fixed_deposit
	RateTiers                  []DepositTier `json:"rate_tiers"`
	TierMethod                 string        `json:"tier_method"` // whole_balance or banded
	Terms                      []DepositTerm `json:"terms"`
	DayCount                   string        `json:"day_count"`
	AccrualBasis               string        `json:"accrual_basis"`
	PayoutFrequency            string        `json:"payout_frequency"`
	WithholdingTaxBPS          int           `json:"withholding_tax_bps"`
	MinOpeningMinor            int64         `json:"min_opening_minor"`
	MinBalanceMinor            int64         `json:"min_balance_minor"`
	MaxBalanceMinor            *int64        `json:"max_balance_minor"`
	WithdrawalsPerMonth        *int          `json:"withdrawals_per_month"`
	ExcessWithdrawalAction     string        `json:"excess_withdrawal_action"` // block, forfeit_interest or fee
	EarlyForfeitBPS            int           `json:"early_forfeit_bps"`
	EarlyPenaltyFixedMinor     int64         `json:"early_penalty_fixed_minor"`
	EarlyPenaltyBPS            int           `json:"early_penalty_bps"`
	DefaultMaturityInstruction string        `json:"default_maturity_instruction"`
	Status                     string        `json:"status"`
	Version                    int           `json:"version"`
	CreatedAt                  time.Time     `json:"created_at"`
	UpdatedAt                  time.Time     `json:"updated_at"`
}

// DepositProductParams creates or updates a product; zero and nil fields are
// left to the engine's defaults (or unchanged). Code and Kind are set on create only.
type DepositProductParams struct {
	Code                       string        `json:"code,omitempty"`
	Name                       string        `json:"name,omitempty"`
	Kind                       string        `json:"kind,omitempty"`
	RateTiers                  []DepositTier `json:"rate_tiers,omitempty"`
	TierMethod                 string        `json:"tier_method,omitempty"`
	Terms                      []DepositTerm `json:"terms,omitempty"`
	DayCount                   string        `json:"day_count,omitempty"`
	AccrualBasis               string        `json:"accrual_basis,omitempty"`
	PayoutFrequency            string        `json:"payout_frequency,omitempty"`
	WithholdingTaxBPS          *int          `json:"withholding_tax_bps,omitempty"`
	MinOpeningMinor            *int64        `json:"min_opening_minor,omitempty"`
	MinBalanceMinor            *int64        `json:"min_balance_minor,omitempty"`
	MaxBalanceMinor            *int64        `json:"max_balance_minor,omitempty"`
	WithdrawalsPerMonth        *int          `json:"withdrawals_per_month,omitempty"`
	ExcessWithdrawalAction     string        `json:"excess_withdrawal_action,omitempty"`
	EarlyForfeitBPS            *int          `json:"early_forfeit_bps,omitempty"`
	EarlyPenaltyFixedMinor     *int64        `json:"early_penalty_fixed_minor,omitempty"`
	EarlyPenaltyBPS            *int          `json:"early_penalty_bps,omitempty"`
	DefaultMaturityInstruction string        `json:"default_maturity_instruction,omitempty"`
	Status                     string        `json:"status,omitempty"`
	Clear                      []string      `json:"clear,omitempty"`
}

type DepositQuoteParams struct {
	ProductID   string `json:"product_id"`
	AmountMinor int64  `json:"amount_minor"`
	Days        int    `json:"days,omitempty"`      // savings: days to project (default 30)
	TermDays    int    `json:"term_days,omitempty"` // fixed deposits
}

type DepositQuote struct {
	ProductID           string `json:"product_id"`
	AmountMinor         int64  `json:"amount_minor"`
	Days                int    `json:"days"`
	RateBPS             int    `json:"rate_bps"`
	GrossInterestMinor  int64  `json:"gross_interest_minor"`
	TaxMinor            int64  `json:"tax_minor"`
	NetInterestMinor    int64  `json:"net_interest_minor"`
	MaturityAmountMinor int64  `json:"maturity_amount_minor"`
}

type AccountInterest struct {
	AccountID       string `json:"account_id"`
	ProductID       string `json:"product_id"`
	ProductName     string `json:"product_name"`
	PayoutFrequency string `json:"payout_frequency"`
	// RateBPS is the rate the current balance earns (a fixed deposit's locked rate).
	RateBPS              int     `json:"rate_bps"`
	AccruedInterestMinor int64   `json:"accrued_interest_minor"`
	AccruedThrough       string  `json:"accrued_through"`
	NextPayoutDay        string  `json:"next_payout_day"`
	ForfeitMonth         *string `json:"forfeit_month"`
	WithdrawalsThisMonth int     `json:"withdrawals_this_month"`
	WithdrawalsPerMonth  *int    `json:"withdrawals_per_month"`
	Payouts              []struct {
		TransactionID string    `json:"transaction_id"`
		GrossMinor    int64     `json:"gross_minor"`
		Narration     string    `json:"narration"`
		GameTime      time.Time `json:"game_time"`
	} `json:"payouts"`
}

type FixedDeposit struct {
	ID                   string     `json:"id"`
	BankID               string     `json:"bank_id"`
	AccountID            string     `json:"account_id"`
	AccountNumber        string     `json:"account_number"`
	PlayerID             string     `json:"player_id"`
	ProductID            string     `json:"product_id"`
	ProductName          string     `json:"product_name"`
	SourceAccountID      string     `json:"source_account_id"`
	PayoutAccountID      string     `json:"payout_account_id"`
	PrincipalMinor       int64      `json:"principal_minor"`
	RateBPS              int        `json:"rate_bps"`
	TermDays             int        `json:"term_days"`
	StartDay             string     `json:"start_day"`
	MaturityDay          string     `json:"maturity_day"`
	MaturityInstruction  string     `json:"maturity_instruction"`
	Status               string     `json:"status"` // active, matured or broken
	TermsCompleted       int        `json:"terms_completed"`
	InterestPaidMinor    int64      `json:"interest_paid_minor"`
	AccruedInterestMinor int64      `json:"accrued_interest_minor"`
	BalanceMinor         int64      `json:"balance_minor"`
	CreatedAt            time.Time  `json:"created_at"`
	ClosedAt             *time.Time `json:"closed_at"`
}

type OpenFixedDepositParams struct {
	SourceAccountID     string `json:"source_account_id"`
	ProductID           string `json:"product_id"`
	TermDays            int    `json:"term_days"`
	AmountMinor         int64  `json:"amount_minor"`
	MaturityInstruction string `json:"maturity_instruction,omitempty"`
	PayoutAccountID     string `json:"payout_account_id,omitempty"` // default the source
}

type OpenFixedDepositResult struct {
	FixedDeposit  FixedDeposit `json:"fixed_deposit"`
	Transaction   Transaction  `json:"transaction"`
	SourceBalance Balance      `json:"source_balance"`
}

type BreakQuote struct {
	FixedDepositID         string `json:"fixed_deposit_id"`
	PrincipalMinor         int64  `json:"principal_minor"`
	AccruedInterestMinor   int64  `json:"accrued_interest_minor"`
	ForfeitedInterestMinor int64  `json:"forfeited_interest_minor"`
	InterestPaidMinor      int64  `json:"interest_paid_minor"`
	TaxMinor               int64  `json:"tax_minor"`
	PenaltyMinor           int64  `json:"penalty_minor"`
	PayoutMinor            int64  `json:"payout_minor"`
	PayoutAccountID        string `json:"payout_account_id"`
}

type BreakFixedDepositResult struct {
	FixedDeposit FixedDeposit `json:"fixed_deposit"`
	Quote        BreakQuote   `json:"quote"`
	Transaction  Transaction  `json:"transaction"`
}

type ListFixedDepositsParams struct {
	PlayerID        string
	SourceAccountID string
	Status          string
	Limit           int
}

// DepositProductsService manages a bank's savings and fixed-deposit products.
type DepositProductsService struct{ t *transport }

func (s *DepositProductsService) List(ctx context.Context, bankID string, opts ...RequestOption) ([]DepositProduct, error) {
	var out struct{ Data []DepositProduct }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/banks/%s/deposit-products", bankID)}, &out, opts)
}

func (s *DepositProductsService) Get(ctx context.Context, bankID, productID string, opts ...RequestOption) (*DepositProduct, error) {
	var out DepositProduct
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/banks/%s/deposit-products/%s", bankID, productID)}, &out, opts)
}

// Create adds a product. Needs bank:write; turn on the bank's "savings" flag to open accounts.
func (s *DepositProductsService) Create(ctx context.Context, bankID string, p DepositProductParams, opts ...RequestOption) (*DepositProduct, error) {
	var out DepositProduct
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/banks/%s/deposit-products", bankID), body: p}, &out, opts)
}

// Update changes a product and bumps its version.
func (s *DepositProductsService) Update(ctx context.Context, bankID, productID string, p DepositProductParams, opts ...RequestOption) (*DepositProduct, error) {
	var out DepositProduct
	return &out, s.t.do(ctx, op{method: http.MethodPatch, path: path("/v1/banks/%s/deposit-products/%s", bankID, productID), body: p}, &out, opts)
}

// Quote projects the interest an amount would earn.
func (s *DepositProductsService) Quote(ctx context.Context, p DepositQuoteParams, opts ...RequestOption) (*DepositQuote, error) {
	if err := checkAmount(p.AmountMinor, true); err != nil {
		return nil, err
	}
	var out DepositQuote
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/deposit-products/quote", body: p}, &out, opts)
}

// Interest returns a savings or fixed-deposit account's rate, accrued interest and payouts.
func (s *AccountsService) Interest(ctx context.Context, accountID string, opts ...RequestOption) (*AccountInterest, error) {
	var out AccountInterest
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/accounts/%s/interest", accountID)}, &out, opts)
}

// FixedDepositsService books, changes and breaks fixed deposits.
type FixedDepositsService struct{ t *transport }

// Open books a fixed deposit at the term's current rate. Needs deposits:write.
func (s *FixedDepositsService) Open(ctx context.Context, p OpenFixedDepositParams, opts ...RequestOption) (*OpenFixedDepositResult, error) {
	if err := checkAmount(p.AmountMinor, true); err != nil {
		return nil, err
	}
	var out OpenFixedDepositResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/fixed-deposits", body: p, idempotent: true}, &out, opts)
}

func (s *FixedDepositsService) List(ctx context.Context, p ListFixedDepositsParams, opts ...RequestOption) ([]FixedDeposit, error) {
	var out struct{ Data []FixedDeposit }
	q := query("player_id", p.PlayerID, "source_account_id", p.SourceAccountID, "status", p.Status, "limit", limitStr(p.Limit))
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/fixed-deposits", query: q}, &out, opts)
}

func (s *FixedDepositsService) Get(ctx context.Context, depositID string, opts ...RequestOption) (*FixedDeposit, error) {
	var out FixedDeposit
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/fixed-deposits/%s", depositID)}, &out, opts)
}

// SetMaturityInstruction changes what happens at maturity.
func (s *FixedDepositsService) SetMaturityInstruction(ctx context.Context, depositID, instruction string, opts ...RequestOption) (*FixedDeposit, error) {
	var out FixedDeposit
	return &out, s.t.do(ctx, op{method: http.MethodPatch, path: path("/v1/fixed-deposits/%s", depositID),
		body: map[string]string{"maturity_instruction": instruction}}, &out, opts)
}

// BreakQuote returns what breaking the deposit early pays now.
func (s *FixedDepositsService) BreakQuote(ctx context.Context, depositID string, opts ...RequestOption) (*BreakQuote, error) {
	var out BreakQuote
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/fixed-deposits/%s/break-quote", depositID)}, &out, opts)
}

// Break ends the deposit early. Pass the quote's PenaltyMinor as maxPenalty
// (nil for no cap); errors.Is(err, ErrPenaltyChanged) if it rose.
func (s *FixedDepositsService) Break(ctx context.Context, depositID string, maxPenalty *int64, opts ...RequestOption) (*BreakFixedDepositResult, error) {
	body := map[string]any{}
	if maxPenalty != nil {
		body["max_penalty_minor"] = *maxPenalty
	}
	var out BreakFixedDepositResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/fixed-deposits/%s/break", depositID), body: body, idempotent: true}, &out, opts)
}
