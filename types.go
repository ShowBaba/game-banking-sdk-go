package gamebank

import "time"

// Amounts are int64 minor units; a currency's Exponent is its decimal places.

type CurrencyBrief struct {
	Code     string  `json:"code"`
	Exponent int     `json:"exponent"`
	Symbol   *string `json:"symbol"`
}

type Currency struct {
	CurrencyBrief
	ID       string `json:"id"`
	Name     string `json:"name"`
	Standard bool   `json:"standard"`
	Status   string `json:"status"`
}

type Account struct {
	ID         string  `json:"id"`
	BankID     string  `json:"bank_id"`
	BankCode   string  `json:"bank_code"`
	OwnerType  string  `json:"owner_type"`
	PlayerID   *string `json:"player_id"`
	MerchantID *string `json:"merchant_id"` // set on merchant accounts
	// ProductID is the deposit product of a savings or fixed-deposit account.
	ProductID *string `json:"product_id"`
	// InterestAccountID receives the account's interest; nil pays the account itself.
	InterestAccountID    *string        `json:"interest_account_id"`
	SystemRole           *string        `json:"system_role"`
	AccountType          string         `json:"account_type"`
	AccountNumber        string         `json:"account_number"`
	AccountName          string         `json:"account_name"`
	Currency             CurrencyBrief  `json:"currency"`
	Status               string         `json:"status"`
	AllowNegativeBalance bool           `json:"allow_negative_balance"`
	PostedBalance        int64          `json:"posted_balance"`
	AvailableBalance     int64          `json:"available_balance"`
	HeldBalance          int64          `json:"held_balance"`
	Version              int64          `json:"version"`
	Metadata             map[string]any `json:"metadata"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

type Bank struct {
	ID            string          `json:"id"`
	Environment   string          `json:"environment"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	BankCode      string          `json:"bank_code"`
	BaseCurrency  Currency        `json:"base_currency"`
	Branding      map[string]any  `json:"branding"`
	FeatureFlags  map[string]bool `json:"feature_flags"`
	ConfigVersion int             `json:"config_version"`
	Status        string          `json:"status"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	// SystemAccounts is set when a bank is created or fetched by ID.
	SystemAccounts []Account `json:"system_accounts,omitempty"`
}

type Balance struct {
	AccountID        string        `json:"account_id"`
	AccountNumber    string        `json:"account_number"`
	Currency         CurrencyBrief `json:"currency"`
	PostedBalance    int64         `json:"posted_balance"`
	AvailableBalance int64         `json:"available_balance"`
	HeldBalance      int64         `json:"held_balance"`
	PostedDisplay    string        `json:"posted_display"`
	AvailableDisplay string        `json:"available_display"`
	Version          int64         `json:"version"`
	AsOf             time.Time     `json:"as_of"`
	AsOfGameTime     *time.Time    `json:"as_of_game_time,omitempty"`
}

type Player struct {
	ID               string         `json:"id"`
	ExternalPlayerID string         `json:"external_player_id"`
	DisplayName      *string        `json:"display_name"`
	Status           string         `json:"status"`
	Metadata         map[string]any `json:"metadata"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type Entry struct {
	ID            string `json:"id"`
	AccountID     string `json:"account_id"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	Side          string `json:"side"`
	AmountMinor   int64  `json:"amount_minor"`
	BalanceAfter  int64  `json:"balance_after"`
	// Kind is "principal", "fee" or "levy". Fee and levy lines carry the rule and its name.
	Kind        string `json:"kind"`
	FeeRuleID   string `json:"fee_rule_id,omitempty"`
	Description string `json:"description,omitempty"`
}

type Transaction struct {
	ID                   string         `json:"id"`
	SessionID            string         `json:"session_id"`
	Type                 string         `json:"type"`
	Status               string         `json:"status"`
	BankID               string         `json:"bank_id"`
	Currency             CurrencyBrief  `json:"currency"`
	AmountMinor          int64          `json:"amount_minor"`
	AmountDisplay        string         `json:"amount_display"`
	Reference            *string        `json:"reference"`
	Narration            *string        `json:"narration"`
	InitiatedBy          Initiator      `json:"initiated_by"`
	RelatedTransactionID *string        `json:"related_transaction_id"`
	ReversedBy           *string        `json:"reversed_by_transaction_id,omitempty"`
	Metadata             map[string]any `json:"metadata"`
	CreatedAt            time.Time      `json:"created_at"`
	PostedAt             *time.Time     `json:"posted_at"`
	GameTime             time.Time      `json:"game_time"`
	Entries              []Entry        `json:"entries"`
}

type Initiator struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type Counterparty struct {
	AccountID     string `json:"account_id"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	BankCode      string `json:"bank_code"`
}

// HistoryItem is one line of an account's statement. Direction is "debit"
// or "credit" from the account holder's point of view.
type HistoryItem struct {
	EntryID       string `json:"entry_id"`
	TransactionID string `json:"transaction_id"`
	SessionID     string `json:"session_id"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Direction     string `json:"direction"`
	AmountMinor   int64  `json:"amount_minor"`
	AmountDisplay string `json:"amount_display"`
	BalanceAfter  int64  `json:"balance_after"`
	// Kind is "principal", "fee" or "levy". A fee or levy line's narration is the fee's name.
	Kind                 string        `json:"kind"`
	Narration            *string       `json:"narration"`
	Reference            *string       `json:"reference"`
	Counterparty         *Counterparty `json:"counterparty"`
	RealTime             time.Time     `json:"real_time"`
	GameTime             time.Time     `json:"game_time"`
	RelatedTransactionID *string       `json:"related_transaction_id"`
}

// HistoryPage is one page of history; pass NextCursor back for the next page.
type HistoryPage struct {
	Data       []HistoryItem `json:"data"`
	NextCursor *string       `json:"next_cursor"`
	HasMore    bool          `json:"has_more"`
}

type Hold struct {
	ID                   string  `json:"id"`
	BankID               string  `json:"bank_id"`
	AccountID            string  `json:"account_id"`
	DestinationAccountID *string `json:"destination_account_id"`
	AmountMinor          int64   `json:"amount_minor"`
	// ReservedFeeMinor is held on top of the amount for the capture's expected fees.
	ReservedFeeMinor     int64      `json:"reserved_fee_minor"`
	CapturedAmountMinor  *int64     `json:"captured_amount_minor"`
	Status               string     `json:"status"`
	Reason               string     `json:"reason"`
	Reference            *string    `json:"reference"`
	ExpiresAtGame        time.Time  `json:"expires_at_game"`
	CaptureTransactionID *string    `json:"capture_transaction_id"`
	CreatedAt            time.Time  `json:"created_at"`
	CreatedGameTime      time.Time  `json:"created_game_time"`
	ClosedAt             *time.Time `json:"closed_at"`
	ClosedGameTime       *time.Time `json:"closed_game_time"`
}

type NameEnquiry struct {
	BankCode      string `json:"bank_code"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	Status        string `json:"status"`
}

type Clock struct {
	Mode     string    `json:"mode"`
	Speed    int       `json:"speed"`
	TimeZone string    `json:"time_zone"`
	GameTime time.Time `json:"game_time"`
	GameDay  string    `json:"game_day"`
	RealTime time.Time `json:"real_time"`
}

// LimitPolicy caps one scope; a nil field means no limit.
type LimitPolicy struct {
	PerTransactionMax *int64 `json:"per_transaction_max"`
	DailyMax          *int64 `json:"daily_max"`
}

// Limit scopes.
const (
	ScopePlayerFunding   = "player_funding"
	ScopeGameFunding     = "game_funding"
	ScopeAccountTransfer = "account_transfer"
)

type Limits struct {
	BankID               string                  `json:"bank_id"`
	Currency             string                  `json:"currency"`
	Limits               map[string]*LimitPolicy `json:"limits"`
	GameDay              string                  `json:"game_day"`
	GameFundingUsedToday int64                   `json:"game_funding_used_today"`
	Note                 string                  `json:"note"`
}

type ReconciliationReport struct {
	StartedAt           time.Time        `json:"started_at"`
	FinishedAt          time.Time        `json:"finished_at"`
	AccountsChecked     int64            `json:"accounts_checked"`
	TransactionsChecked int64            `json:"transactions_checked"`
	Differences         []map[string]any `json:"differences"`
	OK                  bool             `json:"ok"`
}

type FundingResult struct {
	Transaction Transaction `json:"transaction"`
	Balance     Balance     `json:"balance"`
}

type TransferResult struct {
	Transaction        Transaction `json:"transaction"`
	SourceBalance      Balance     `json:"source_balance"`
	DestinationBalance *Balance    `json:"destination_balance,omitempty"`
	// Fees and levies charged to the sender on top of the amount.
	Fees           []FeeCharge `json:"fees"`
	FeesTotalMinor int64       `json:"fees_total_minor"`
	// TotalDebitMinor is the amount plus fees.
	TotalDebitMinor int64 `json:"total_debit_minor"`
}

type HoldResult struct {
	Hold    Hold    `json:"hold"`
	Balance Balance `json:"balance"`
}

type CaptureResult struct {
	Hold               Hold        `json:"hold"`
	Transaction        Transaction `json:"transaction"`
	SourceBalance      Balance     `json:"source_balance"`
	DestinationBalance *Balance    `json:"destination_balance,omitempty"`
	Fees               []FeeCharge `json:"fees"`
	FeesTotalMinor     int64       `json:"fees_total_minor"`
	TotalDebitMinor    int64       `json:"total_debit_minor"`
}

type AdvanceClockResult struct {
	Clock            Clock     `json:"clock"`
	PreviousGameTime time.Time `json:"previous_game_time"`
	Jobs             struct {
		GameDay      string   `json:"game_day"`
		HoldsExpired int      `json:"holds_expired"`
		JobsRun      []string `json:"jobs_run"`
	} `json:"jobs"`
}

// ---- request parameters (empty fields are omitted, so the engine's defaults apply) ----

type UpsertPlayerParams struct {
	ExternalPlayerID string         `json:"external_player_id"`
	DisplayName      string         `json:"display_name,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type OpenAccountParams struct {
	BankID string `json:"bank_id"`
	// PlayerID opens a player account; MerchantID a merchant account, which receives card payments.
	PlayerID   string `json:"player_id,omitempty"`
	MerchantID string `json:"merchant_id,omitempty"`
	// ProductID makes a savings account that earns interest under that product.
	ProductID         string         `json:"product_id,omitempty"`
	InterestAccountID string         `json:"interest_account_id,omitempty"`
	AccountType       string         `json:"account_type,omitempty"`
	AccountName       string         `json:"account_name,omitempty"`
	Currency          string         `json:"currency,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type ListAccountsParams struct {
	BankID        string
	PlayerID      string
	MerchantID    string
	IncludeSystem bool
	Limit         int
}

type UpdateAccountStatusParams struct {
	Status string `json:"status"` // active, frozen or closed
	Reason string `json:"reason,omitempty"`
}

type FundingParams struct {
	AccountID   string `json:"account_id"`
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason"`
	// Reference is your own unique ID for this issuance.
	Reference       string         `json:"reference"`
	Currency        string         `json:"currency,omitempty"`
	SourceAccountID string         `json:"source_account_id,omitempty"`
	Narration       string         `json:"narration,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type TransferParams struct {
	SourceAccountID          string         `json:"source_account_id"`
	AmountMinor              int64          `json:"amount_minor"`
	DestinationAccountID     string         `json:"destination_account_id,omitempty"`
	DestinationAccountNumber string         `json:"destination_account_number,omitempty"`
	DestinationBankCode      string         `json:"destination_bank_code,omitempty"`
	BankID                   string         `json:"bank_id,omitempty"`
	Currency                 string         `json:"currency,omitempty"`
	Reference                string         `json:"reference,omitempty"`
	Narration                string         `json:"narration,omitempty"`
	Metadata                 map[string]any `json:"metadata,omitempty"`
	// MaxFeeMinor, when set, refuses the transfer with ErrFeeChanged if its fees
	// now total more, e.g. the FeesTotalMinor of a quote the player accepted.
	MaxFeeMinor *int64 `json:"max_fee_minor,omitempty"`
}

type PlaceHoldParams struct {
	AccountID                string     `json:"account_id"`
	AmountMinor              int64      `json:"amount_minor"`
	Reason                   string     `json:"reason"`
	Reference                string     `json:"reference,omitempty"`
	DestinationAccountID     string     `json:"destination_account_id,omitempty"`
	DestinationAccountNumber string     `json:"destination_account_number,omitempty"`
	ExpiresAtGame            *time.Time `json:"expires_at_game,omitempty"`
	ExpiresInGameSeconds     int64      `json:"expires_in_game_seconds,omitempty"`
	// MaxFeeMinor caps the capture fee reserved now (ErrFeeChanged if higher).
	MaxFeeMinor *int64 `json:"max_fee_minor,omitempty"`
}

type CaptureHoldParams struct {
	// AmountMinor is the amount to capture; 0 captures the whole hold.
	AmountMinor              int64          `json:"amount_minor,omitempty"`
	DestinationAccountID     string         `json:"destination_account_id,omitempty"`
	DestinationAccountNumber string         `json:"destination_account_number,omitempty"`
	Narration                string         `json:"narration,omitempty"`
	Reference                string         `json:"reference,omitempty"`
	Metadata                 map[string]any `json:"metadata,omitempty"`
	// MaxFeeMinor refuses the capture with ErrFeeChanged if its fees now total more.
	MaxFeeMinor *int64 `json:"max_fee_minor,omitempty"`
}

type ReverseParams struct {
	Reason    string `json:"reason"`
	Narration string `json:"narration,omitempty"`
}

type CreateBankParams struct {
	Name         string          `json:"name"`
	BaseCurrency string          `json:"base_currency"`
	Slug         string          `json:"slug,omitempty"`
	BankCode     string          `json:"bank_code,omitempty"`
	Branding     map[string]any  `json:"branding,omitempty"`
	FeatureFlags map[string]bool `json:"feature_flags,omitempty"`
}

type UpdateBankParams struct {
	Name         string          `json:"name,omitempty"`
	Branding     map[string]any  `json:"branding,omitempty"`
	FeatureFlags map[string]bool `json:"feature_flags,omitempty"`
	Status       string          `json:"status,omitempty"`
}

type CreateCurrencyParams struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Exponent int    `json:"exponent"`
	Symbol   string `json:"symbol,omitempty"`
}

type ConfigureClockParams struct {
	Mode     string     `json:"mode,omitempty"`
	Speed    int        `json:"speed,omitempty"`
	TimeZone string     `json:"time_zone,omitempty"`
	GameTime *time.Time `json:"game_time,omitempty"`
}

type AdvanceClockParams struct {
	Seconds int64      `json:"seconds,omitempty"`
	Minutes int64      `json:"minutes,omitempty"`
	Hours   int64      `json:"hours,omitempty"`
	Days    int64      `json:"days,omitempty"`
	To      *time.Time `json:"to,omitempty"`
}

// ---- fees and charges ----

// Fee events, kinds, models and states.
const (
	FeeEventTransfer           = "transfer"
	FeeEventHoldCapture        = "hold_capture"
	FeeEventAccountMaintenance = "account_maintenance"

	FeeKindFee  = "fee"  // credits the bank's fee_income account
	FeeKindLevy = "levy" // credits levies_payable: money held for someone else

	FeeModelFixed      = "fixed"
	FeeModelPercentage = "percentage"
	FeeModelCombined   = "combined"

	FeeStateInEffect  = "in_effect"
	FeeStateScheduled = "scheduled"
	FeeStateEnded     = "ended"
	FeeStateInactive  = "inactive"
)

// FeeRule is a fee or levy a bank charges. Amounts are minor units; times are game time.
type FeeRule struct {
	ID             string     `json:"id"`
	BankID         string     `json:"bank_id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Event          string     `json:"event"`
	Kind           string     `json:"kind"`
	Model          string     `json:"model"`
	FixedMinor     int64      `json:"fixed_minor"`
	PercentageBPS  int64      `json:"percentage_bps"` // 100 = 1%
	MinFeeMinor    *int64     `json:"min_fee_minor"`
	MaxFeeMinor    *int64     `json:"max_fee_minor"`
	MinAmountMinor *int64     `json:"min_amount_minor"`
	MaxAmountMinor *int64     `json:"max_amount_minor"`
	Payer          string     `json:"payer"`
	CreditAccount  string     `json:"credit_account"`
	Status         string     `json:"status"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveTo    *time.Time `json:"effective_to"`
	Version        int        `json:"version"`
	// State is where the rule stands at the current game time; only in_effect rules charge.
	State     string    `json:"state"`
	Warnings  []string  `json:"warnings"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateFeeRuleParams adds a fee. Leave optional pointers nil for "no limit".
type CreateFeeRuleParams struct {
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Event          string     `json:"event"`
	Model          string     `json:"model"`
	Kind           string     `json:"kind,omitempty"`
	FixedMinor     int64      `json:"fixed_minor,omitempty"`
	PercentageBPS  int64      `json:"percentage_bps,omitempty"`
	MinFeeMinor    *int64     `json:"min_fee_minor,omitempty"`
	MaxFeeMinor    *int64     `json:"max_fee_minor,omitempty"`
	MinAmountMinor *int64     `json:"min_amount_minor,omitempty"`
	MaxAmountMinor *int64     `json:"max_amount_minor,omitempty"`
	Status         string     `json:"status,omitempty"`
	EffectiveFrom  *time.Time `json:"effective_from,omitempty"`
	EffectiveTo    *time.Time `json:"effective_to,omitempty"`
	// StartNow starts the fee at the current game time.
	StartNow bool `json:"start_now,omitempty"`
}

// UpdateFeeRuleParams changes a fee; nil fields keep their value. Clear resets
// fields to "no limit", e.g. []string{"max_fee_minor"}.
type UpdateFeeRuleParams struct {
	Name           *string    `json:"name,omitempty"`
	Kind           *string    `json:"kind,omitempty"`
	Model          *string    `json:"model,omitempty"`
	FixedMinor     *int64     `json:"fixed_minor,omitempty"`
	PercentageBPS  *int64     `json:"percentage_bps,omitempty"`
	MinFeeMinor    *int64     `json:"min_fee_minor,omitempty"`
	MaxFeeMinor    *int64     `json:"max_fee_minor,omitempty"`
	MinAmountMinor *int64     `json:"min_amount_minor,omitempty"`
	MaxAmountMinor *int64     `json:"max_amount_minor,omitempty"`
	Status         *string    `json:"status,omitempty"`
	EffectiveFrom  *time.Time `json:"effective_from,omitempty"`
	EffectiveTo    *time.Time `json:"effective_to,omitempty"`
	StartNow       bool       `json:"start_now,omitempty"`
	Clear          []string   `json:"clear,omitempty"`
}

// FeeCharge is one fee or levy charged on a payment.
type FeeCharge struct {
	FeeRuleID      string `json:"fee_rule_id"`
	FeeRuleVersion int    `json:"fee_rule_version"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	AmountMinor    int64  `json:"amount_minor"`
	CreditAccount  string `json:"credit_account"`
}

// QuoteFeesParams asks what a transfer (default) or hold capture would cost.
type QuoteFeesParams struct {
	BankID      string `json:"bank_id"`
	AmountMinor int64  `json:"amount_minor"`
	Event       string `json:"event,omitempty"`
}

type FeeQuote struct {
	BankID          string      `json:"bank_id"`
	Event           string      `json:"event"`
	AmountMinor     int64       `json:"amount_minor"`
	Fees            []FeeCharge `json:"fees"`
	FeesTotalMinor  int64       `json:"fees_total_minor"`
	TotalDebitMinor int64       `json:"total_debit_minor"`
	GameTime        time.Time   `json:"game_time"`
}

// BankBooks is the bank's own ledger, from which its profit and loss derives.
type BankBooks struct {
	BankID             string `json:"bank_id"`
	Currency           string `json:"currency"`
	FeeIncomeMinor     int64  `json:"fee_income_minor"`
	LeviesPayableMinor int64  `json:"levies_payable_minor"`
	// InterestExpenseMinor is interest paid to savers (negative).
	InterestExpenseMinor int64 `json:"interest_expense_minor"`
	// AccruedInterestMinor is interest earned by savers, not yet paid.
	AccruedInterestMinor int64 `json:"accrued_interest_minor"`
	// FixedDepositsMinor is money in active fixed deposits.
	FixedDepositsMinor int64 `json:"fixed_deposits_minor"`
	Accounts           []struct {
		SystemRole     string `json:"system_role"`
		AccountName    string `json:"account_name"`
		AccountNumber  string `json:"account_number"`
		BalanceMinor   int64  `json:"balance_minor"`
		BalanceDisplay string `json:"balance_display"`
	} `json:"accounts"`
	AsOf time.Time `json:"as_of"`
}

// ---- management ----

type Developer struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ContactEmail string    `json:"contact_email"`
	Status       string    `json:"status"`
	HasPassword  bool      `json:"has_password"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Game struct {
	ID          string    `json:"id"`
	DeveloperID string    `json:"developer_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type APIKey struct {
	ID          string `json:"id"`
	GameID      string `json:"game_id"`
	Environment string `json:"environment"`
	Label       string `json:"label"`
	Prefix      string `json:"prefix"`
	// Secret is set only in the response that created the key.
	Secret     string     `json:"secret,omitempty"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type DeveloperCredential struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Prefix     string     `json:"prefix"`
	Secret     string     `json:"secret,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type AuditLog struct {
	ID            string         `json:"id"`
	DeveloperID   *string        `json:"developer_id"`
	GameID        *string        `json:"game_id"`
	Environment   *string        `json:"environment"`
	ActorType     string         `json:"actor_type"`
	ActorID       string         `json:"actor_id"`
	Action        string         `json:"action"`
	TargetType    *string        `json:"target_type"`
	TargetID      *string        `json:"target_id"`
	CorrelationID *string        `json:"correlation_id"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     time.Time      `json:"created_at"`
}

// UpdateGameParams renames a game or changes its slug or status; empty fields keep their value.
type UpdateGameParams struct {
	Name   string `json:"name,omitempty"`
	Slug   string `json:"slug,omitempty"`
	Status string `json:"status,omitempty"`
}

type IssueAPIKeyParams struct {
	Environment string `json:"environment"`
	Label       string `json:"label,omitempty"`
	// Scopes default to bank:read, account:read, account:write, transfer:create.
	Scopes    []string   `json:"scopes,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// API key scopes.
const (
	ScopeBankRead           = "bank:read"
	ScopeBankWrite          = "bank:write"
	ScopeAccountRead        = "account:read"
	ScopeAccountWrite       = "account:write"
	ScopeTransferCreate     = "transfer:create"
	ScopeFundingCreate      = "funding:create"
	ScopeTransactionReverse = "transaction:reverse"
	ScopeLimitsWrite        = "limits:write"
	ScopeWebhooksRead       = "webhooks:read"
	ScopeWebhooksWrite      = "webhooks:write"
	ScopeMerchantsWrite     = "merchants:write"
	ScopeCardsRead          = "cards:read"
	ScopeCardsWrite         = "cards:write"
	ScopeCardsCharge        = "cards:charge"
	ScopeDepositsWrite      = "deposits:write"
)
