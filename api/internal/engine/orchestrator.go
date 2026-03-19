package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/protobuf/types/known/durationpb"
)

// TaskEnqueuer enqueues a job for worker execution.
type TaskEnqueuer interface {
	Enqueue(ctx context.Context, jobID string, jobToken string) error
}

// CloudTasksConfig holds the configuration for Cloud Tasks.
type CloudTasksConfig struct {
	Project   string
	Location  string
	Queue     string
	WorkerURL string
}

// CloudTasksEnqueuer dispatches jobs via Google Cloud Tasks.
type CloudTasksEnqueuer struct {
	client *cloudtasks.Client
	cfg    CloudTasksConfig
}

// NewCloudTasksEnqueuer creates a Cloud Tasks enqueuer.
func NewCloudTasksEnqueuer(ctx context.Context, cfg CloudTasksConfig) (*CloudTasksEnqueuer, error) {
	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create cloud tasks client: %w", err)
	}
	return &CloudTasksEnqueuer{client: client, cfg: cfg}, nil
}

// Enqueue creates an HTTP task targeting the worker's /run endpoint.
// Retry policy (max attempts: 3, min backoff: 10s, max backoff: 300s,
// max doublings: 4) is configured at the queue level.
func (e *CloudTasksEnqueuer) Enqueue(ctx context.Context, jobID string, jobToken string) error {
	queuePath := fmt.Sprintf("projects/%s/locations/%s/queues/%s",
		e.cfg.Project, e.cfg.Location, e.cfg.Queue)

	body, err := json.Marshal(map[string]string{
		"job_id":    jobID,
		"job_token": jobToken,
	})
	if err != nil {
		return fmt.Errorf("marshal task body: %w", err)
	}

	req := &taskspb.CreateTaskRequest{
		Parent: queuePath,
		Task: &taskspb.Task{
			// Task-level timeout: 20 minutes (aligned with watchdog).
			DispatchDeadline: durationpb.New(20 * time.Minute),
			MessageType: &taskspb.Task_HttpRequest{
				HttpRequest: &taskspb.HttpRequest{
					HttpMethod: taskspb.HttpMethod_POST,
					Url:        e.cfg.WorkerURL + "/run",
					Headers:    map[string]string{"Content-Type": "application/json"},
					Body:       body,
				},
			},
		},
	}

	if _, err := e.client.CreateTask(ctx, req); err != nil {
		return fmt.Errorf("create cloud task: %w", err)
	}
	return nil
}

// Close closes the underlying Cloud Tasks client.
func (e *CloudTasksEnqueuer) Close() error {
	return e.client.Close()
}
