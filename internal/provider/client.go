package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const defaultAPIBaseURL = "https://api.github.com"

// APIClientOption is a functional option for NewAPIClient.
type APIClientOption func(*APIClient)

// WithHTTPClient overrides the HTTP client used for GitHub API calls.
func WithHTTPClient(c *http.Client) APIClientOption {
	return func(a *APIClient) { a.httpClient = c }
}

// WithBaseURL overrides the GitHub API base URL (for testing).
func WithBaseURL(url string) APIClientOption {
	return func(a *APIClient) { a.apiBaseURL = url }
}

// WebhookValidator is the minimal interface required by consumers that only validate webhooks.
type WebhookValidator interface {
	ValidateWebhook(ctx context.Context, payload []byte, signature string, eventType string) (WebhookEvent, error)
}

// Worker is the full provider interface. It embeds WebhookValidator and adds the
// operations needed by the worker container (clone, upload).
type Worker interface {
	WebhookValidator
	GenerateCloneToken(ctx context.Context, installationID int64) (string, error)
	CloneRepo(ctx context.Context, repoURL, ref, token, dst string) error
	ListRepos(ctx context.Context, installationID int64) ([]Repo, error)
}

// APIClient holds provider credentials. Construct with NewAPIClient.
type APIClient struct {
	appID         int64
	privateKey    []byte
	webhookSecret string
	httpClient    *http.Client
	apiBaseURL    string
}

// NewAPIClient returns a configured APIClient. Returns an error if privateKey or
// webhookSecret is empty — both are required for correct operation at startup.
func NewAPIClient(appID int64, privateKey []byte, webhookSecret string, opts ...APIClientOption) (*APIClient, error) {
	if len(privateKey) == 0 {
		return nil, errors.New("privateKey must not be empty")
	}
	if webhookSecret == "" {
		return nil, errors.New("webhookSecret must not be empty")
	}
	a := &APIClient{
		appID:         appID,
		privateKey:    privateKey,
		webhookSecret: webhookSecret,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		apiBaseURL:    defaultAPIBaseURL,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// ClientHandler wraps an APIClient and satisfies the Worker interface.
type ClientHandler struct {
	client *APIClient
}

// Compile-time guard: ClientHandler must satisfy Worker.
var _ Worker = (*ClientHandler)(nil)

func NewClientHandler(client *APIClient) *ClientHandler {
	if client == nil {
		panic("NewClientHandler: client must not be nil")
	}
	return &ClientHandler{client: client}
}

// ValidateWebhook validates the HMAC-SHA256 signature of a GitHub webhook payload,
// then parses it into a WebhookEvent. eventType is the value of the X-GitHub-Event header.
// Uses constant-time comparison to prevent timing attacks.
func (h *ClientHandler) ValidateWebhook(ctx context.Context, payload []byte, signature string, eventType string) (WebhookEvent, error) {
	mac := hmac.New(sha256.New, []byte(h.client.webhookSecret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return WebhookEvent{}, ErrInvalidSignature
	}

	p, err := parsePayload(payload)
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	return WebhookEvent{
		EventType:      eventType,
		Action:         ActionType(p.Action),
		RepoID:         p.Repository.ID,
		RepoName:       p.Repository.FullName,
		CloneURL:       p.Repository.CloneURL,
		Branch:         branchFromRef(p.Ref),
		CommitSHA:      p.After,
		InstallationID: p.Installation.ID,
	}, nil
}

// generateAppJWT creates a signed RS256 JWT for authenticating as the GitHub App.
// GitHub requires iat to be slightly in the past to account for clock skew.
func (h *ClientHandler) generateAppJWT() (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(h.client.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	iat := time.Now().Add(-60 * time.Second)
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(iat),
		ExpiresAt: jwt.NewNumericDate(iat.Add(10 * time.Minute)),
		Issuer:    strconv.FormatInt(h.client.appID, 10),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}
	return signed, nil
}

// GenerateCloneToken exchanges a GitHub App JWT for a short-lived installation access token.
func (h *ClientHandler) GenerateCloneToken(ctx context.Context, installationID int64) (string, error) {
	appJWT, err := h.generateAppJWT()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", h.client.apiBaseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := h.client.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	if result.Token == "" {
		return "", errors.New("GitHub API returned empty token")
	}

	return result.Token, nil
}

// CloneRepo performs a shallow clone of repoURL at the given ref into dst.
// token is injected into the URL as an installation access token; pass empty string for unauthenticated clones.
func (h *ClientHandler) CloneRepo(ctx context.Context, repoURL, ref, token, dst string) error {
	authURL, err := buildAuthURL(repoURL, token)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", ref, authURL, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %w: %s", err, out)
	}
	return nil
}

func (h *ClientHandler) ListRepos(ctx context.Context, installationID int64) ([]Repo, error) {
	return nil, nil // TODO
}

// Test push
