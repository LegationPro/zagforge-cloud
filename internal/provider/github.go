package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalidSignature is returned by ValidateWebhook when the HMAC signature does not match.
var ErrInvalidSignature = errors.New("invalid webhook signature")

type ActionType string

// WebhookEvent is the parsed result of a validated webhook payload.
type WebhookEvent struct {
	EventType      string // value of X-GitHub-Event header
	Action         ActionType
	RepoID         int64
	RepoName       string // "owner/repo"
	CloneURL       string // HTTPS clone URL from the payload
	Branch         string
	CommitSHA      string
	InstallationID int64
}

type Repo struct {
	ID            int64
	FullName      string
	DefaultBranch string
}

// pushPayload is the minimal GitHub webhook payload structure we need.
type pushPayload struct {
	Ref    string `json:"ref"`
	After  string `json:"after"`
	Action string `json:"action"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

func parsePayload(payload []byte) (pushPayload, error) {
	var p pushPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return pushPayload{}, err
	}
	return p, nil
}

// branchFromRef strips the "refs/heads/" prefix from a Git ref.
// If the ref is not a branch ref, it is returned as-is.
func branchFromRef(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

// buildAuthURL injects an installation access token into an HTTPS repo URL.
// For file:// URLs (e.g. in tests) the token is ignored and the URL is returned unchanged.
func buildAuthURL(repoURL, token string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repo URL: %w", err)
	}
	if token != "" && u.Scheme == "https" {
		u.User = url.UserPassword("x-access-token", token)
	}
	return u.String(), nil
}
