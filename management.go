package gamebank

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Management is the setup and CI client, authenticated with a developer
// credential (gbe_dev_…) or a sign-in session token.
type Management struct {
	t           *transport
	Games       *GamesService
	APIKeys     *APIKeysService
	Credentials *DeveloperCredentialsService
}

// NewManagement builds a management client. Without options it reads GBE_API_KEY and GBE_BASE_URL.
func NewManagement(opts ...Option) (*Management, error) {
	t, err := newTransport(opts)
	if err != nil {
		return nil, err
	}
	return &Management{t: t, Games: &GamesService{t}, APIKeys: &APIKeysService{t}, Credentials: &DeveloperCredentialsService{t}}, nil
}

// Me returns the developer this credential belongs to.
func (m *Management) Me(ctx context.Context, opts ...RequestOption) (*Developer, error) {
	var out Developer
	return &out, m.t.do(ctx, op{method: http.MethodGet, path: "/v1/developer"}, &out, opts)
}

// AuditLogs lists a game's audit log, newest first. environment may be "".
func (m *Management) AuditLogs(ctx context.Context, gameID, environment string, limit int, before *time.Time, opts ...RequestOption) ([]AuditLog, error) {
	q := url.Values{}
	if environment != "" {
		q.Set("environment", environment)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if before != nil {
		q.Set("before", before.UTC().Format(time.RFC3339Nano))
	}
	var out struct{ Data []AuditLog }
	return out.Data, m.t.do(ctx, op{method: http.MethodGet, path: path("/v1/games/%s/audit-logs", gameID), query: q}, &out, opts)
}

// Game returns a Client acting on one of your games with your developer
// credential, to configure banks, currencies, limits and the clock. Moving
// money still needs a game API key.
func (m *Management) Game(gameID, environment string) *Client {
	return newClient(m.t.withHeaders(map[string]string{"X-Game-Id": gameID, "X-Environment": environment}))
}

type GamesService struct{ t *transport }

// Create registers a game; slug may be "" to derive it from the name.
func (s *GamesService) Create(ctx context.Context, name, slug string, opts ...RequestOption) (*Game, error) {
	body := map[string]string{"name": name}
	if slug != "" {
		body["slug"] = slug
	}
	var out Game
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/games", body: body}, &out, opts)
}

func (s *GamesService) List(ctx context.Context, opts ...RequestOption) ([]Game, error) {
	var out struct{ Data []Game }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/games"}, &out, opts)
}

func (s *GamesService) Get(ctx context.Context, gameID string, opts ...RequestOption) (*Game, error) {
	var out Game
	return &out, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/games/%s", gameID)}, &out, opts)
}

// Update renames a game, changes its slug, or suspends or reactivates it.
func (s *GamesService) Update(ctx context.Context, gameID string, p UpdateGameParams, opts ...RequestOption) (*Game, error) {
	var out Game
	return &out, s.t.do(ctx, op{method: http.MethodPatch, path: path("/v1/games/%s", gameID), body: p}, &out, opts)
}

// SetStatus suspends or reactivates a game ("active" or "suspended").
func (s *GamesService) SetStatus(ctx context.Context, gameID, status string, opts ...RequestOption) (*Game, error) {
	return s.Update(ctx, gameID, UpdateGameParams{Status: status}, opts...)
}

// Delete deletes a game permanently: its API keys are revoked at once and it
// leaves the developer API. Ledger and audit history are kept.
func (s *GamesService) Delete(ctx context.Context, gameID string, opts ...RequestOption) error {
	return s.t.do(ctx, op{method: http.MethodDelete, path: path("/v1/games/%s", gameID)}, nil, opts)
}

type APIKeysService struct{ t *transport }

// Issue creates a game API key. Secret is set in the result once; store it on your servers.
func (s *APIKeysService) Issue(ctx context.Context, gameID string, p IssueAPIKeyParams, opts ...RequestOption) (*APIKey, error) {
	var out struct {
		APIKey APIKey `json:"api_key"`
	}
	if err := s.t.do(ctx, op{method: http.MethodPost, path: path("/v1/games/%s/api-keys", gameID), body: p}, &out, opts); err != nil {
		return nil, err
	}
	return &out.APIKey, nil
}

func (s *APIKeysService) List(ctx context.Context, gameID string, opts ...RequestOption) ([]APIKey, error) {
	var out struct{ Data []APIKey }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: path("/v1/games/%s/api-keys", gameID)}, &out, opts)
}

// Revoke revokes a key immediately.
func (s *APIKeysService) Revoke(ctx context.Context, gameID, keyID string, opts ...RequestOption) error {
	return s.t.do(ctx, op{method: http.MethodDelete, path: path("/v1/games/%s/api-keys/%s", gameID, keyID)}, nil, opts)
}

type DeveloperCredentialsService struct{ t *transport }

func (s *DeveloperCredentialsService) List(ctx context.Context, opts ...RequestOption) ([]DeveloperCredential, error) {
	var out struct{ Data []DeveloperCredential }
	return out.Data, s.t.do(ctx, op{method: http.MethodGet, path: "/v1/developer/credentials"}, &out, opts)
}

// Issue creates a long-lived developer credential for scripts and CI; expiresAt may be nil.
func (s *DeveloperCredentialsService) Issue(ctx context.Context, label string, expiresAt *time.Time, opts ...RequestOption) (*DeveloperCredential, error) {
	body := map[string]any{"label": label}
	if expiresAt != nil {
		body["expires_at"] = expiresAt.UTC().Format(time.RFC3339)
	}
	var out DeveloperCredential
	return &out, s.t.do(ctx, op{method: http.MethodPost, path: "/v1/developer/credentials", body: body}, &out, opts)
}

func (s *DeveloperCredentialsService) Revoke(ctx context.Context, credentialID string, opts ...RequestOption) error {
	return s.t.do(ctx, op{method: http.MethodDelete, path: path("/v1/developer/credentials/%s", credentialID)}, nil, opts)
}
