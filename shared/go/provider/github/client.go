package github

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

var GithubApiVersion = "2026-03-10"

// ClientHandler wraps an APIClient and satisfies the provider.Worker interface.
type ClientHandler struct {
	client *APIClient
}

// Compile-time guard: ClientHandler must satisfy provider.Worker.
var _ Worker = (*ClientHandler)(nil)

func NewClientHandler(client *APIClient) (*ClientHandler, error) {
	if client == nil {
		return nil, fmt.Errorf("NewClientHandler: client must not be nil")
	}
	return &ClientHandler{client: client}, nil
}

// ValidateWebhook validates the HMAC-SHA256 signature of a GitHub webhook payload,
// then parses it into a provider.WebhookEvent. eventType is the value of the X-GitHub-Event header.
// Uses constant-time comparison to prevent timing attacks.
func (h *ClientHandler) ValidateWebhook(ctx context.Context, payload []byte, signature string, eventType string) (WebhookEvent, error) {
	mac := hmac.New(sha256.New, []byte(h.client.webhookSecret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return WebhookEvent{}, ErrInvalidSignature
	}

	p, err := ParsePayload(payload)
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	return WebhookEvent{
		EventType:      eventType,
		Action:         ActionType(p.Action),
		RepoID:         p.Repository.ID,
		RepoName:       p.Repository.FullName,
		CloneURL:       p.Repository.CloneURL,
		Branch:         BranchFromRef(p.Ref),
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
	req.Header.Set("X-GitHub-Api-Version", GithubApiVersion)

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
	authURL, err := BuildAuthURL(repoURL, token)
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
