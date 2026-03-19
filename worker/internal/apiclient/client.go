package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/jobtoken"
)

var (
	ErrStartFailed    = errors.New("start callback failed")
	ErrCompleteFailed = errors.New("complete callback failed")
)

// StartRequest is the body sent when the worker begins a job.
type StartRequest struct {
	JobID string `json:"job_id"`
}

// StartResponse contains clone info returned by the API.
type StartResponse struct {
	CommitSHA      string `json:"commit_sha"`
	Branch         string `json:"branch"`
	RepoFullName   string `json:"repo_full_name"`
	CloneToken     string `json:"clone_token"`
	InstallationID int64  `json:"installation_id"`
}

// CompleteRequest is the body sent when the worker finishes a job.
type CompleteRequest struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	ErrorMessage  string `json:"error_message,omitempty"`
	SnapshotPath  string `json:"snapshot_path,omitempty"`
	ZigzagVersion string `json:"zigzag_version,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
}

// Client calls the API's internal job callback endpoints.
type Client struct {
	baseURL string
	signer  *jobtoken.Signer
	http    *http.Client
	log     *zap.Logger
}

// NewClient creates an API callback client.
func NewClient(baseURL string, signer *jobtoken.Signer, log *zap.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		signer:  signer,
		http:    &http.Client{},
		log:     log,
	}
}

func (c *Client) closeBody(body io.ReadCloser) {
	if err := body.Close(); err != nil {
		c.log.Warn("failed to close response body", zap.Error(err))
	}
}

// Start calls POST /internal/jobs/start and returns clone info for the job.
func (c *Client) Start(ctx context.Context, jobID string) (StartResponse, error) {
	body, err := json.Marshal(StartRequest{JobID: jobID})
	if err != nil {
		return StartResponse{}, fmt.Errorf("marshal start request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/jobs/start", bytes.NewReader(body))
	if err != nil {
		return StartResponse{}, fmt.Errorf("create start request: %w", err)
	}

	token := c.signer.Sign(jobID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return StartResponse{}, fmt.Errorf("start request: %w", err)
	}
	defer c.closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return StartResponse{}, fmt.Errorf("%w: status %d: %s", ErrStartFailed, resp.StatusCode, respBody)
	}

	var result StartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StartResponse{}, fmt.Errorf("decode start response: %w", err)
	}

	return result, nil
}

// Complete calls POST /internal/jobs/complete to report job result.
func (c *Client) Complete(ctx context.Context, req CompleteRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal complete request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/jobs/complete", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create complete request: %w", err)
	}

	token := c.signer.Sign(req.JobID)
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("complete request: %w", err)
	}
	defer c.closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d: %s", ErrCompleteFailed, resp.StatusCode, respBody)
	}

	return nil
}
