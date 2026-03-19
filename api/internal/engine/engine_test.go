package engine_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/LegationPro/zagforge/api/internal/engine"
)

func TestNoopEnqueuer_returnsNil(t *testing.T) {
	e := engine.NewNoopEnqueuer(zap.NewNop())
	if err := e.Enqueue(context.Background(), "job-123", "token-abc"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestNoopEnqueuer_satisfiesInterface(t *testing.T) {
	var _ engine.TaskEnqueuer = engine.NewNoopEnqueuer(zap.NewNop())
}
