package engine

import (
	"context"

	"go.uber.org/zap"
)

// NoopEnqueuer is a dev-mode enqueuer that logs and does nothing.
// The poller picks up jobs from the database instead.
type NoopEnqueuer struct {
	log *zap.Logger
}

// NewNoopEnqueuer creates a no-op enqueuer for local development.
func NewNoopEnqueuer(log *zap.Logger) *NoopEnqueuer {
	return &NoopEnqueuer{log: log}
}

// Enqueue logs the job and returns nil. The poller will pick it up.
func (n *NoopEnqueuer) Enqueue(_ context.Context, jobID string, _ string) error {
	n.log.Debug("noop enqueue (poller will pick up)", zap.String("job_id", jobID))
	return nil
}
