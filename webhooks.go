package gamebank

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Webhook event types an endpoint can subscribe to ("*" means all).
const (
	EventAccountDebited  = "account.debited"
	EventAccountCredited = "account.credited"
	EventWebhookTest     = "webhook.test"
)

// WebhookSignatureHeader carries a delivery's timestamp and signatures.
const WebhookSignatureHeader = "GBE-Signature"

// WebhookEndpoint is a registered receiver of signed events.
type WebhookEndpoint struct {
	ID          string   `json:"id"`
	URL         string   `json:"url"`
	Description *string  `json:"description"`
	EventTypes  []string `json:"event_types"`
	// Status is "active" or "disabled"; the engine disables an endpoint after three days of failed deliveries.
	Status         string  `json:"status"`
	DisabledReason *string `json:"disabled_reason"`
	SecretPrefix   string  `json:"secret_prefix"`
	// Secret is set only when the endpoint is created or its secret rotated.
	Secret string `json:"secret,omitempty"`
	// PreviousSecretExpiresAt: until then deliveries are also signed with the previous secret.
	PreviousSecretExpiresAt *time.Time `json:"previous_secret_expires_at"`
	FailingSince            *time.Time `json:"failing_since"`
	LastSuccessAt           *time.Time `json:"last_success_at"`
	LastFailureAt           *time.Time `json:"last_failure_at"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type CreateWebhookEndpointParams struct {
	// URL must be https (the sandbox also accepts http); private and loopback addresses are refused.
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	// EventTypes defaults to ["*"].
	EventTypes []string `json:"event_types,omitempty"`
}

// UpdateWebhookEndpointParams changes an endpoint; nil fields are unchanged.
type UpdateWebhookEndpointParams struct {
	URL         *string  `json:"url,omitempty"`
	Description *string  `json:"description,omitempty"`
	EventTypes  []string `json:"event_types,omitempty"`
	// Status "active" re-enables an endpoint; "disabled" pauses it.
	Status *string `json:"status,omitempty"`
}

// WebhookEvent is the body of a delivery, and an item of the events feed.
// Deduplicate on ID: delivery is at least once.
type WebhookEvent struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	CreatedAt   time.Time  `json:"created_at"`
	GameTime    *time.Time `json:"game_time"`
	GameID      string     `json:"game_id"`
	Environment string     `json:"environment"`
	// Data is an AccountAlert for account.* events; decode it with Alert.
	Data json.RawMessage `json:"data"`
}

// Alert decodes the data of an account.debited or account.credited event.
func (e *WebhookEvent) Alert() (*AccountAlert, error) {
	if e.Type != EventAccountDebited && e.Type != EventAccountCredited {
		return nil, fmt.Errorf("gamebank: %s is not an account alert", e.Type)
	}
	var a AccountAlert
	return &a, json.Unmarshal(e.Data, &a)
}

// AlertCharge is a fee or levy line on an alert: charged on a debit, refunded on a credit.
type AlertCharge struct {
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	FeeRuleID   *string `json:"fee_rule_id"`
	AmountMinor int64   `json:"amount_minor"`
}

// AccountAlert is the data of an account.debited or account.credited event.
type AccountAlert struct {
	AccountID            string  `json:"account_id"`
	AccountNumber        string  `json:"account_number"`
	AccountName          string  `json:"account_name"`
	PlayerID             *string `json:"player_id"`
	BankID               string  `json:"bank_id"`
	BankCode             string  `json:"bank_code"`
	TransactionID        string  `json:"transaction_id"`
	SessionID            string  `json:"session_id"`
	TransactionType      string  `json:"transaction_type"`
	RelatedTransactionID *string `json:"related_transaction_id"`
	// AmountMinor is the principal (0 for a fee-only transaction).
	AmountMinor int64         `json:"amount_minor"`
	Charges     []AlertCharge `json:"charges"`
	// TotalMinor is principal plus charges: what left (or reached) the account.
	TotalMinor   int64         `json:"total_minor"`
	TotalDisplay string        `json:"total_display"`
	Currency     CurrencyBrief `json:"currency"`
	BalanceAfter struct {
		PostedMinor    int64 `json:"posted_minor"`
		AvailableMinor int64 `json:"available_minor"`
	} `json:"balance_after"`
	Narration *string `json:"narration"`
	Reference *string `json:"reference"`
	// Counterparty is the other side; customer account numbers are masked to the last four digits.
	Counterparty *struct {
		Type          string `json:"type"`
		AccountName   string `json:"account_name"`
		AccountNumber string `json:"account_number"`
		BankCode      string `json:"bank_code"`
	} `json:"counterparty"`
	GameTime time.Time `json:"game_time"`
	// Sequence increases with every ledger entry: keep the alert with the highest for the latest balance.
	Sequence int64 `json:"sequence"`
}

// WebhookDelivery is one event's delivery to one endpoint.
type WebhookDelivery struct {
	ID          string `json:"id"`
	EventID     string `json:"event_id"`
	EventType   string `json:"event_type"`
	EndpointID  string `json:"endpoint_id"`
	EndpointURL string `json:"endpoint_url"`
	// State is pending, delivering, delivered or failed.
	State         string     `json:"state"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	// LastStatus is the endpoint's HTTP status on the last attempt.
	LastStatus     *int       `json:"last_status"`
	LastError      *string    `json:"last_error"`
	LastResponseMs *int       `json:"last_response_ms"`
	DeliveredAt    *time.Time `json:"delivered_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type WebhookEventPage struct {
	Data       []WebhookEvent `json:"data"`
	NextCursor *string        `json:"next_cursor"`
	HasMore    bool           `json:"has_more"`
}

type ListWebhookEventsParams struct {
	Type      string
	AccountID string
	Limit     int
	Cursor    string
}

// WebhooksService manages signed event notifications (debit and credit alerts).
// Verify deliveries with VerifyWebhook.
type WebhooksService struct {
	Endpoints *WebhookEndpointsService
	Events    *WebhookEventsService
}

type WebhookEndpointsService struct{ t *transport }

// Create registers an endpoint. The result's Secret is shown only now: store
// it and pass it to VerifyWebhook. Needs webhooks:write.
func (s *WebhookEndpointsService) Create(ctx context.Context, p CreateWebhookEndpointParams, opts ...RequestOption) (*WebhookEndpoint, error) {
	var out WebhookEndpoint
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/webhooks/endpoints", body: p}, &out, opts)
}

func (s *WebhookEndpointsService) List(ctx context.Context, opts ...RequestOption) ([]WebhookEndpoint, error) {
	var out struct{ Data []WebhookEndpoint }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/webhooks/endpoints"}, &out, opts)
}

func (s *WebhookEndpointsService) Get(ctx context.Context, endpointID string, opts ...RequestOption) (*WebhookEndpoint, error) {
	var out WebhookEndpoint
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/webhooks/endpoints/%s", endpointID)}, &out, opts)
}

// Update changes the URL, description or event types, or pauses (Status "disabled") and re-enables it.
func (s *WebhookEndpointsService) Update(ctx context.Context, endpointID string, p UpdateWebhookEndpointParams, opts ...RequestOption) (*WebhookEndpoint, error) {
	var out WebhookEndpoint
	return &out, s.t.do(ctx, op{method: http.MethodPatch, path: path("/v1/webhooks/endpoints/%s", endpointID), body: p}, &out, opts)
}

func (s *WebhookEndpointsService) Delete(ctx context.Context, endpointID string, opts ...RequestOption) error {
	return s.t.do(ctx, op{method: http.MethodDelete, path: path("/v1/webhooks/endpoints/%s", endpointID)}, nil, opts)
}

// RotateSecret issues a new secret (in the result). The old one keeps signing for 24 hours.
func (s *WebhookEndpointsService) RotateSecret(ctx context.Context, endpointID string, opts ...RequestOption) (*WebhookEndpoint, error) {
	var out WebhookEndpoint
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/webhooks/endpoints/%s/rotate-secret", endpointID)}, &out, opts)
}

// Test sends a webhook.test event now and returns how that attempt went.
func (s *WebhookEndpointsService) Test(ctx context.Context, endpointID string, opts ...RequestOption) (*WebhookDelivery, error) {
	var out WebhookDelivery
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/webhooks/endpoints/%s/test", endpointID)}, &out, opts)
}

type WebhookEventsService struct{ t *transport }

// List returns one page of the event log, newest first (kept 30 days). It is also a polling feed.
func (s *WebhookEventsService) List(ctx context.Context, p ListWebhookEventsParams, opts ...RequestOption) (*WebhookEventPage, error) {
	q := url.Values{}
	if p.Type != "" {
		q.Set("type", p.Type)
	}
	if p.AccountID != "" {
		q.Set("account_id", p.AccountID)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	var out WebhookEventPage
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/webhooks/events", query: q}, &out, opts)
}

// Iterate yields every event matching the filter, newest first, fetching pages as needed.
func (s *WebhookEventsService) Iterate(ctx context.Context, p ListWebhookEventsParams, opts ...RequestOption) iter.Seq2[WebhookEvent, error] {
	return func(yield func(WebhookEvent, error) bool) {
		p.Cursor = ""
		for {
			page, err := s.List(ctx, p, opts...)
			if err != nil {
				yield(WebhookEvent{}, err)
				return
			}
			for _, ev := range page.Data {
				if !yield(ev, nil) {
					return
				}
			}
			if page.NextCursor == nil || *page.NextCursor == "" {
				return
			}
			p.Cursor = *page.NextCursor
		}
	}
}

func (s *WebhookEventsService) Get(ctx context.Context, eventID string, opts ...RequestOption) (*WebhookEvent, error) {
	var out WebhookEvent
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/webhooks/events/%s", eventID)}, &out, opts)
}

// Deliveries lists the event's deliveries, one per endpoint, with attempts and the last error.
func (s *WebhookEventsService) Deliveries(ctx context.Context, eventID string, opts ...RequestOption) ([]WebhookDelivery, error) {
	var out struct{ Data []WebhookDelivery }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/webhooks/events/%s/deliveries", eventID)}, &out, opts)
}

// Redeliver sends the event again, to one endpoint (endpointID) or, with "",
// every subscribed one. Retries start over.
func (s *WebhookEventsService) Redeliver(ctx context.Context, eventID, endpointID string, opts ...RequestOption) ([]WebhookDelivery, error) {
	body := map[string]string{}
	if endpointID != "" {
		body["endpoint_id"] = endpointID
	}
	var out struct{ Data []WebhookDelivery }
	return out.Data, s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/webhooks/events/%s/redeliver", eventID), body: body}, &out, opts)
}

// ---- verification ---------------------------------------------------------------

// ErrWebhookVerification matches every WebhookVerificationError with errors.Is.
var ErrWebhookVerification = errors.New("gamebank: webhook verification failed")

// WebhookVerificationError means the request did not come from the engine, or
// was replayed or tampered with. Answer 400.
type WebhookVerificationError struct {
	// Reason is malformed_header, timestamp_out_of_tolerance, no_matching_signature or invalid_payload.
	Reason  string
	Message string
}

func (e *WebhookVerificationError) Error() string        { return "gamebank: " + e.Message }
func (e *WebhookVerificationError) Is(target error) bool { return target == ErrWebhookVerification }

// WebhookVerifier checks deliveries. The zero Tolerance means 5 minutes; Now defaults to time.Now.
type WebhookVerifier struct {
	// Secrets: the endpoint secret, or several while you roll a new one out.
	Secrets   []string
	Tolerance time.Duration
	Now       func() time.Time
}

// VerifyWebhook checks a delivery with one secret and the default tolerance:
//
//	body, _ := io.ReadAll(r.Body)
//	event, err := gamebank.VerifyWebhook(body, r.Header.Get(gamebank.WebhookSignatureHeader), secret)
//	if err != nil { http.Error(w, "bad signature", http.StatusBadRequest); return }
//
// Pass the raw body, before any JSON decoding.
func VerifyWebhook(payload []byte, signatureHeader, secret string) (*WebhookEvent, error) {
	return WebhookVerifier{Secrets: []string{secret}}.Verify(payload, signatureHeader)
}

// Verify checks that a signature matches one of the secrets and that its
// timestamp is within the tolerance, then decodes the event.
func (v WebhookVerifier) Verify(payload []byte, signatureHeader string) (*WebhookEvent, error) {
	var ts int64
	var sigs [][]byte
	for _, part := range strings.Split(signatureHeader, ",") {
		k, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				ts = n
			}
		case "v1":
			if b, err := hex.DecodeString(val); err == nil {
				sigs = append(sigs, b)
			}
		}
	}
	if ts <= 0 || len(sigs) == 0 {
		return nil, &WebhookVerificationError{Reason: "malformed_header", Message: "missing or malformed " + WebhookSignatureHeader + " header"}
	}
	tolerance := v.Tolerance
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	now := time.Now
	if v.Now != nil {
		now = v.Now
	}
	if d := now().Sub(time.Unix(ts, 0)); d > tolerance || d < -tolerance {
		return nil, &WebhookVerificationError{Reason: "timestamp_out_of_tolerance", Message: "the signature's timestamp is outside the tolerance"}
	}
	matched := false
	for _, secret := range v.Secrets {
		m := hmac.New(sha256.New, []byte(secret))
		fmt.Fprintf(m, "%d.", ts)
		m.Write(payload)
		want := m.Sum(nil)
		for _, s := range sigs {
			if hmac.Equal(s, want) {
				matched = true
			}
		}
	}
	if !matched {
		return nil, &WebhookVerificationError{Reason: "no_matching_signature", Message: "no signature matches the secret"}
	}
	var ev WebhookEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, &WebhookVerificationError{Reason: "invalid_payload", Message: "the body is not a JSON event"}
	}
	return &ev, nil
}
