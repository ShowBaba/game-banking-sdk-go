package gamebank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Merchant is a business in the game (an NPC shop or a player-run store).
type Merchant struct {
	ID                 string         `json:"id"`
	ExternalMerchantID string         `json:"external_merchant_id"`
	Name               string         `json:"name"`
	Category           string         `json:"category"`
	OwnerPlayerID      *string        `json:"owner_player_id"`
	Status             string         `json:"status"` // active or suspended (card payments declined)
	Metadata           map[string]any `json:"metadata"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type CreateMerchantParams struct {
	// ExternalMerchantID is your own ID; creating it again returns the existing merchant.
	ExternalMerchantID string         `json:"external_merchant_id"`
	Name               string         `json:"name"`
	Category           string         `json:"category,omitempty"` // default "general"
	OwnerPlayerID      string         `json:"owner_player_id,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

type UpdateMerchantParams struct {
	Name     *string        `json:"name,omitempty"`
	Category *string        `json:"category,omitempty"`
	Status   *string        `json:"status,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ListMerchantsParams struct {
	Status             string
	OwnerPlayerID      string
	ExternalMerchantID string
	Limit              int
}

type CardProgram struct {
	ID             string `json:"id"`
	BankID         string `json:"bank_id"`
	Name           string `json:"name"`
	BIN            string `json:"bin"`
	ValidityMonths int    `json:"validity_months"`
	// AuthExpirySeconds is how long an authorisation holds funds, in game seconds.
	AuthExpirySeconds int64     `json:"auth_expiry_seconds"`
	PerTxnLimitMinor  *int64    `json:"per_txn_limit_minor"`
	DailyLimitMinor   *int64    `json:"daily_limit_minor"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CardProgramParams creates or updates a program; nil fields are unchanged (or defaulted).
type CardProgramParams struct {
	Name              *string  `json:"name,omitempty"`
	BIN               *string  `json:"bin,omitempty"` // create only
	ValidityMonths    *int     `json:"validity_months,omitempty"`
	AuthExpirySeconds *int64   `json:"auth_expiry_seconds,omitempty"`
	PerTxnLimitMinor  *int64   `json:"per_txn_limit_minor,omitempty"`
	DailyLimitMinor   *int64   `json:"daily_limit_minor,omitempty"`
	Status            *string  `json:"status,omitempty"`
	Clear             []string `json:"clear,omitempty"`
}

// Card statuses.
const (
	CardActive  = "active"
	CardBlocked = "blocked"
	CardLost    = "lost"
	CardStolen  = "stolen"
	CardExpired = "expired"
	CardClosed  = "closed"
)

type Card struct {
	ID           string  `json:"id"`
	BankID       string  `json:"bank_id"`
	AccountID    string  `json:"account_id"`
	PlayerID     string  `json:"player_id"`
	ProgramID    string  `json:"program_id"`
	MaskedNumber string  `json:"masked_number"`
	Last4        string  `json:"last4"`
	ExpiryMonth  int     `json:"expiry_month"`
	ExpiryYear   int     `json:"expiry_year"` // valid through the end of ExpiryMonth of game time
	NameOnCard   string  `json:"name_on_card"`
	Status       string  `json:"status"`
	StatusReason *string `json:"status_reason"`
	// The card's own limits; nil uses the program's.
	PerTxnLimitMinor          *int64         `json:"per_txn_limit_minor"`
	DailyLimitMinor           *int64         `json:"daily_limit_minor"`
	EffectivePerTxnLimitMinor *int64         `json:"effective_per_txn_limit_minor"`
	EffectiveDailyLimitMinor  *int64         `json:"effective_daily_limit_minor"`
	Metadata                  map[string]any `json:"metadata"`
	StatusChangedAt           time.Time      `json:"status_changed_at"`
	CreatedAt                 time.Time      `json:"created_at"`
	UpdatedAt                 time.Time      `json:"updated_at"`
}

type IssueCardParams struct {
	AccountID        string         `json:"account_id"` // a player account
	ProgramID        string         `json:"program_id,omitempty"`
	NameOnCard       string         `json:"name_on_card,omitempty"`
	PerTxnLimitMinor *int64         `json:"per_txn_limit_minor,omitempty"`
	DailyLimitMinor  *int64         `json:"daily_limit_minor,omitempty"`
	MaxFeeMinor      *int64         `json:"max_fee_minor,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// IssueCardResult carries CardNumber and CVV, shown only here.
type IssueCardResult struct {
	Card           Card        `json:"card"`
	CardNumber     string      `json:"card_number"`
	CVV            string      `json:"cvv"`
	Fees           []FeeCharge `json:"fees"`
	FeesTotalMinor int64       `json:"fees_total_minor"`
	Balance        Balance     `json:"balance"`
}

type UpdateCardParams struct {
	NameOnCard       *string        `json:"name_on_card,omitempty"`
	PerTxnLimitMinor *int64         `json:"per_txn_limit_minor,omitempty"`
	DailyLimitMinor  *int64         `json:"daily_limit_minor,omitempty"`
	Clear            []string       `json:"clear,omitempty"` // back to the program's limit
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type ListCardsParams struct {
	AccountID string
	PlayerID  string
	Status    string
	Limit     int
}

// Decline codes on a declined authorisation.
const (
	DeclineInvalidExpiry     = "invalid_expiry"
	DeclineInvalidCVV        = "invalid_cvv"
	DeclineCardBlocked       = "card_blocked"
	DeclineCardLost          = "card_lost"
	DeclineCardStolen        = "card_stolen"
	DeclineCardExpired       = "card_expired"
	DeclineCardClosed        = "card_closed"
	DeclineAccountFrozen     = "account_frozen"
	DeclineAccountClosed     = "account_closed"
	DeclineMerchantInactive  = "merchant_inactive"
	DeclineExceedsTxnLimit   = "exceeds_transaction_limit"
	DeclineExceedsDailyLimit = "exceeds_daily_limit"
	DeclineInsufficientFunds = "insufficient_funds"
)

type CardRefund struct {
	ID            string    `json:"id"`
	TransactionID string    `json:"transaction_id"`
	AmountMinor   int64     `json:"amount_minor"`
	Reason        *string   `json:"reason"`
	GameTime      time.Time `json:"game_time"`
	CreatedAt     time.Time `json:"created_at"`
}

type CardAuthorization struct {
	ID        string `json:"id"`
	CardID    string `json:"card_id"`
	CardLast4 string `json:"card_last4"`
	BankID    string `json:"bank_id"`
	AccountID string `json:"account_id"`
	Merchant  struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
	} `json:"merchant"`
	MerchantAccountID string `json:"merchant_account_id"`
	// Status is approved, declined, captured, released, expired or refunded.
	Status               string        `json:"status"`
	DeclineCode          *string       `json:"decline_code"`
	DeclineMessage       *string       `json:"decline_message"`
	AmountMinor          int64         `json:"amount_minor"`
	CapturedMinor        int64         `json:"captured_minor"`
	RefundedMinor        int64         `json:"refunded_minor"`
	Currency             CurrencyBrief `json:"currency"`
	HoldID               *string       `json:"hold_id"`
	ReservedFeeMinor     int64         `json:"reserved_fee_minor"`
	ExpiresAtGame        *time.Time    `json:"expires_at_game"`
	CaptureTransactionID *string       `json:"capture_transaction_id"`
	Reference            *string       `json:"reference"`
	Description          *string       `json:"description"`
	Refunds              []CardRefund  `json:"refunds"`
	GameTime             time.Time     `json:"game_time"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	ClosedAt             *time.Time    `json:"closed_at"`
}

// Declined reports whether the payment was declined, and why.
func (a CardAuthorization) Declined() (code string, declined bool) {
	if a.Status != "declined" || a.DeclineCode == nil {
		return "", false
	}
	return *a.DeclineCode, true
}

type AuthorizeCardParams struct {
	// The card: CardID, or CardNumber with ExpiryMonth and ExpiryYear (and optionally CVV).
	CardID      string `json:"card_id,omitempty"`
	CardNumber  string `json:"card_number,omitempty"`
	ExpiryMonth int    `json:"expiry_month,omitempty"`
	ExpiryYear  int    `json:"expiry_year,omitempty"`
	CVV         string `json:"cvv,omitempty"`
	// A merchant account in the card's bank.
	MerchantAccountID     string `json:"merchant_account_id,omitempty"`
	MerchantAccountNumber string `json:"merchant_account_number,omitempty"`
	AmountMinor           int64  `json:"amount_minor"`
	Reference             string `json:"reference,omitempty"`
	Description           string `json:"description,omitempty"`
	MaxFeeMinor           *int64 `json:"max_fee_minor,omitempty"`
	// Hold reserves the money instead of charging it now: the payment waits
	// for Capture or Release, or expires. By default an approved payment is
	// captured in full in the same call.
	Hold bool `json:"-"`
}

// MarshalJSON sends Hold as the engine's "capture": false.
func (p AuthorizeCardParams) MarshalJSON() ([]byte, error) {
	type plain AuthorizeCardParams
	out := struct {
		plain
		Capture *bool `json:"capture,omitempty"`
	}{plain: plain(p)}
	if p.Hold {
		out.Capture = new(bool)
	}
	return json.Marshal(out)
}

// AuthorizeCardResult: a decline is a result, not an error; see Authorization.Declined.
type AuthorizeCardResult struct {
	Authorization  CardAuthorization `json:"authorization"`
	Fees           []FeeCharge       `json:"fees"`
	FeesTotalMinor int64             `json:"fees_total_minor"`
	// Balance is the cardholder's account after the call.
	Balance Balance `json:"balance"`
	// Set when the payment was captured (approved without Hold).
	Transaction     *Transaction `json:"transaction,omitempty"`
	MerchantBalance *Balance     `json:"merchant_balance,omitempty"`
	TotalDebitMinor int64        `json:"total_debit_minor,omitempty"`
}

type CaptureCardParams struct {
	AmountMinor int64  `json:"amount_minor,omitempty"` // 0 captures everything; the rest is released
	MaxFeeMinor *int64 `json:"max_fee_minor,omitempty"`
}

type CaptureCardResult struct {
	Authorization   CardAuthorization `json:"authorization"`
	Transaction     Transaction       `json:"transaction"`
	CardBalance     Balance           `json:"card_balance"`
	MerchantBalance Balance           `json:"merchant_balance"`
	Fees            []FeeCharge       `json:"fees"`
	FeesTotalMinor  int64             `json:"fees_total_minor"`
	TotalDebitMinor int64             `json:"total_debit_minor"`
}

type ReleaseCardResult struct {
	Authorization CardAuthorization `json:"authorization"`
	Balance       Balance           `json:"balance"`
}

type RefundCardParams struct {
	AmountMinor int64  `json:"amount_minor,omitempty"` // 0 refunds the rest
	Reason      string `json:"reason,omitempty"`
}

type RefundCardResult struct {
	Refund          CardRefund        `json:"refund"`
	Authorization   CardAuthorization `json:"authorization"`
	Transaction     Transaction       `json:"transaction"`
	CardBalance     Balance           `json:"card_balance"`
	MerchantBalance Balance           `json:"merchant_balance"`
}

type ListCardAuthorizationsParams struct {
	CardID     string
	MerchantID string
	AccountID  string
	Status     string
	Limit      int
}

func query(kv ...string) url.Values {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			q.Set(kv[i], kv[i+1])
		}
	}
	return q
}

func limitStr(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// MerchantsService manages merchants. Open a merchant's account with
// Accounts.Open and MerchantID.
type MerchantsService struct{ t *transport }

// Create registers a merchant; repeat calls with the same ExternalMerchantID return it. Needs merchants:write.
func (s *MerchantsService) Create(ctx context.Context, p CreateMerchantParams, opts ...RequestOption) (*Merchant, error) {
	var out Merchant
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/merchants", body: p, retrySafe: true}, &out, opts)
}

func (s *MerchantsService) List(ctx context.Context, p ListMerchantsParams, opts ...RequestOption) ([]Merchant, error) {
	var out struct{ Data []Merchant }
	q := query("status", p.Status, "owner_player_id", p.OwnerPlayerID, "external_merchant_id", p.ExternalMerchantID, "limit", limitStr(p.Limit))
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/merchants", query: q}, &out, opts)
}

func (s *MerchantsService) Get(ctx context.Context, merchantID string, opts ...RequestOption) (*Merchant, error) {
	var out Merchant
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/merchants/%s", merchantID)}, &out, opts)
}

// Update renames, recategorises, suspends or reactivates a merchant.
func (s *MerchantsService) Update(ctx context.Context, merchantID string, p UpdateMerchantParams, opts ...RequestOption) (*Merchant, error) {
	var out Merchant
	return &out, s.t.do(ctx, op{method: http.MethodPatch, path: path("/v1/merchants/%s", merchantID), body: p}, &out, opts)
}

// CardProgramsService manages a bank's card programs.
type CardProgramsService struct{ t *transport }

func (s *CardProgramsService) List(ctx context.Context, bankID string, opts ...RequestOption) ([]CardProgram, error) {
	var out struct{ Data []CardProgram }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/banks/%s/card-programs", bankID)}, &out, opts)
}

func (s *CardProgramsService) Get(ctx context.Context, bankID, programID string, opts ...RequestOption) (*CardProgram, error) {
	var out CardProgram
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/banks/%s/card-programs/%s", bankID, programID)}, &out, opts)
}

// Create adds a program. Turn on the bank's "cards" feature flag to issue cards. Needs bank:write.
func (s *CardProgramsService) Create(ctx context.Context, bankID string, p CardProgramParams, opts ...RequestOption) (*CardProgram, error) {
	var out CardProgram
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/banks/%s/card-programs", bankID), body: p}, &out, opts)
}

func (s *CardProgramsService) Update(ctx context.Context, bankID, programID string, p CardProgramParams, opts ...RequestOption) (*CardProgram, error) {
	var out CardProgram
	return &out, s.t.do(ctx, op{method: http.MethodPatch, path: path("/v1/banks/%s/card-programs/%s", bankID, programID), body: p}, &out, opts)
}

// CardsService issues and controls debit cards.
type CardsService struct{ t *transport }

// Issue creates a card on a player account, charging any card_issuance fee.
// CardNumber and CVV are in the result once. Needs cards:write.
func (s *CardsService) Issue(ctx context.Context, p IssueCardParams, opts ...RequestOption) (*IssueCardResult, error) {
	var out IssueCardResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/cards", body: p, idempotent: true}, &out, opts)
}

func (s *CardsService) List(ctx context.Context, p ListCardsParams, opts ...RequestOption) ([]Card, error) {
	var out struct{ Data []Card }
	q := query("account_id", p.AccountID, "player_id", p.PlayerID, "status", p.Status, "limit", limitStr(p.Limit))
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/cards", query: q}, &out, opts)
}

func (s *CardsService) Get(ctx context.Context, cardID string, opts ...RequestOption) (*Card, error) {
	var out Card
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/cards/%s", cardID)}, &out, opts)
}

// Update changes the name on the card or its own limits.
func (s *CardsService) Update(ctx context.Context, cardID string, p UpdateCardParams, opts ...RequestOption) (*Card, error) {
	var out Card
	return &out, s.t.do(ctx, op{method: http.MethodPatch, path: path("/v1/cards/%s", cardID), body: p}, &out, opts)
}

func (s *CardsService) status(ctx context.Context, cardID, action, reason string, opts []RequestOption) (*Card, error) {
	var out Card
	body := map[string]string{}
	if reason != "" {
		body["reason"] = reason
	}
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/cards/%s/"+action, cardID), body: body}, &out, opts)
}

// Block pauses the card: new payments are declined (card_blocked).
func (s *CardsService) Block(ctx context.Context, cardID, reason string, opts ...RequestOption) (*Card, error) {
	return s.status(ctx, cardID, "block", reason, opts)
}

func (s *CardsService) Unblock(ctx context.Context, cardID string, opts ...RequestOption) (*Card, error) {
	return s.status(ctx, cardID, "unblock", "", opts)
}

// ReportLost reports the card "lost" or "stolen": final, and no replacement
// is issued. Payments approved before the report still capture.
func (s *CardsService) ReportLost(ctx context.Context, cardID, reason string, opts ...RequestOption) (*Card, error) {
	return s.status(ctx, cardID, "report-lost", reason, opts)
}

func (s *CardsService) Close(ctx context.Context, cardID, reason string, opts ...RequestOption) (*Card, error) {
	return s.status(ctx, cardID, "close", reason, opts)
}

// CardAuthorizationsService runs card payments.
type CardAuthorizationsService struct{ t *transport }

// Authorize pays a merchant: an approved payment is captured at once, or with
// Hold only reserved for a later Capture or Release. A decline is not an
// error: check result.Authorization.Declined(). Needs cards:charge.
func (s *CardAuthorizationsService) Authorize(ctx context.Context, p AuthorizeCardParams, opts ...RequestOption) (*AuthorizeCardResult, error) {
	if err := checkAmount(p.AmountMinor, true); err != nil {
		return nil, err
	}
	var out AuthorizeCardResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/card-authorizations", body: p, idempotent: true}, &out, opts)
}

func (s *CardAuthorizationsService) List(ctx context.Context, p ListCardAuthorizationsParams, opts ...RequestOption) ([]CardAuthorization, error) {
	var out struct{ Data []CardAuthorization }
	q := query("card_id", p.CardID, "merchant_id", p.MerchantID, "account_id", p.AccountID, "status", p.Status, "limit", limitStr(p.Limit))
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/card-authorizations", query: q}, &out, opts)
}

func (s *CardAuthorizationsService) Get(ctx context.Context, authorizationID string, opts ...RequestOption) (*CardAuthorization, error) {
	var out CardAuthorization
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/card-authorizations/%s", authorizationID)}, &out, opts)
}

// Capture settles all or part of an approved payment into the merchant account; the rest is released.
func (s *CardAuthorizationsService) Capture(ctx context.Context, authorizationID string, p CaptureCardParams, opts ...RequestOption) (*CaptureCardResult, error) {
	if err := checkAmount(p.AmountMinor, false); err != nil {
		return nil, err
	}
	var out CaptureCardResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/card-authorizations/%s/capture", authorizationID), body: p, idempotent: true}, &out, opts)
}

// Release cancels an approved payment and frees its funds.
func (s *CardAuthorizationsService) Release(ctx context.Context, authorizationID, reason string, opts ...RequestOption) (*ReleaseCardResult, error) {
	body := map[string]string{}
	if reason != "" {
		body["reason"] = reason
	}
	var out ReleaseCardResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/card-authorizations/%s/release", authorizationID), body: body, idempotent: true}, &out, opts)
}

// Refund refunds all (AmountMinor 0) or part of a captured payment from the merchant account. Fees are not refunded.
func (s *CardAuthorizationsService) Refund(ctx context.Context, authorizationID string, p RefundCardParams, opts ...RequestOption) (*RefundCardResult, error) {
	if err := checkAmount(p.AmountMinor, false); err != nil {
		return nil, err
	}
	var out RefundCardResult
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/card-authorizations/%s/refunds", authorizationID), body: p, idempotent: true}, &out, opts)
}
