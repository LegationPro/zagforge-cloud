package poller_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/runner"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
	"github.com/LegationPro/zagforge-mvp-impl/worker/internal/worker/executor"
	"github.com/LegationPro/zagforge-mvp-impl/worker/internal/worker/poller"
)

type noopCloner struct{}

func (n *noopCloner) GenerateCloneToken(_ context.Context, _ int64) (string, error) {
	return "", nil
}

func (n *noopCloner) CloneRepo(_ context.Context, _, _, _, _ string) error {
	return nil
}

type mockClaimer struct {
	claimErr    error
	job         store.Job
	repo        store.GetRepoForJobRow
	repoErr     error
	claimCount  atomic.Int64
	statusCalls atomic.Int64
}

func (m *mockClaimer) ClaimJob(_ context.Context) (store.Job, error) {
	m.claimCount.Add(1)
	return m.job, m.claimErr
}

func (m *mockClaimer) GetRepoForJob(_ context.Context, _ pgtype.UUID) (store.GetRepoForJobRow, error) {
	return m.repo, m.repoErr
}

func (m *mockClaimer) UpdateJobStatus(_ context.Context, _ store.UpdateJobStatusParams) error {
	m.statusCalls.Add(1)
	return nil
}

func TestPoller_Run_shutsDownCleanly(t *testing.T) {
	claimer := &mockClaimer{claimErr: pgx.ErrNoRows}
	r := runner.New(&noopCloner{}, runner.Config{}, zap.NewNop())
	exec := executor.NewExecutor(nil, r, zap.NewNop())
	p := poller.NewPoller(claimer, r, exec, zap.NewNop(), 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- p.Run(ctx)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("poller did not shut down within timeout")
	}
}

func TestPoller_Run_pollsAtInterval(t *testing.T) {
	claimer := &mockClaimer{claimErr: pgx.ErrNoRows}
	r := runner.New(&noopCloner{}, runner.Config{}, zap.NewNop())
	exec := executor.NewExecutor(nil, r, zap.NewNop())
	p := poller.NewPoller(claimer, r, exec, zap.NewNop(), 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- p.Run(ctx)
	}()

	time.Sleep(180 * time.Millisecond)
	cancel()
	<-done

	count := claimer.claimCount.Load()
	if count < 2 {
		t.Fatalf("expected at least 2 polls, got %d", count)
	}
}

func TestPoller_Run_repoNotFound_marksJobFailed(t *testing.T) {
	claimer := &mockClaimer{
		job: store.Job{
			ID:     pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
			Branch: "main",
		},
		repoErr: pgx.ErrNoRows,
	}

	r := runner.New(&noopCloner{}, runner.Config{}, zap.NewNop())
	exec := executor.NewExecutor(nil, r, zap.NewNop())
	p := poller.NewPoller(claimer, r, exec, zap.NewNop(), 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- p.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if claimer.statusCalls.Load() < 1 {
		t.Fatal("expected UpdateJobStatus to be called when repo not found")
	}
}
