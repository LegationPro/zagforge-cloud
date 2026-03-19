package poller

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/runner"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
	"github.com/LegationPro/zagforge-mvp-impl/worker/internal/worker/executor"
)

// JobClaimer is the subset of store.Queries the poller needs.
type JobClaimer interface {
	ClaimJob(ctx context.Context) (store.Job, error)
	GetRepoForJob(ctx context.Context, id pgtype.UUID) (store.GetRepoForJobRow, error)
	UpdateJobStatus(ctx context.Context, arg store.UpdateJobStatusParams) error
}

// Poller claims queued jobs from the database and dispatches them for execution.
type Poller struct {
	claimer        JobClaimer
	runner         *runner.Runner
	executor       *executor.Executor
	log            *zap.Logger
	interval       time.Duration
	maxConcurrency int
}

func NewPoller(claimer JobClaimer, runner *runner.Runner, executor *executor.Executor, log *zap.Logger, interval time.Duration, maxConcurrency int) *Poller {
	return &Poller{
		claimer:        claimer,
		runner:         runner,
		executor:       executor,
		log:            log,
		interval:       interval,
		maxConcurrency: maxConcurrency,
	}
}

// Run starts the poll loop. It blocks until ctx is cancelled, then drains in-flight jobs.
func (p *Poller) Run(ctx context.Context) error {
	p.log.Info("worker started",
		zap.Duration("interval", p.interval),
		zap.Int("max_concurrency", p.maxConcurrency),
	)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info("shutting down worker", zap.Int64("in_flight_jobs", p.runner.InFlight()))
			if err := p.runner.Drain(2*time.Minute, 5*time.Second); err != nil {
				return err
			}
			p.log.Info("worker stopped")
			return nil
		case <-ticker.C:
			p.pollBatch(ctx)
		}
	}
}

// pollBatch claims jobs up to the number of free slots in the pool.
func (p *Poller) pollBatch(ctx context.Context) {
	slots := int64(p.maxConcurrency) - p.runner.InFlight()
	for range slots {
		if ctx.Err() != nil {
			return
		}
		if err := p.claimOne(ctx); err != nil {
			if err == errNoJobs {
				return // queue empty, stop claiming
			}
			p.log.Error("poll error", zap.Error(err))
			return
		}
	}
}

var errNoJobs = fmt.Errorf("no queued jobs")

func (p *Poller) claimOne(ctx context.Context) error {
	job, err := p.claimer.ClaimJob(ctx)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errNoJobs
		}
		return fmt.Errorf("claim job: %w", err)
	}

	repo, err := p.claimer.GetRepoForJob(ctx, job.ID)
	if err != nil {
		if statusErr := p.claimer.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:           job.ID,
			Status:       store.JobStatusFailed,
			ErrorMessage: pgtype.Text{String: "repo not found for job", Valid: true},
		}); statusErr != nil {
			p.log.Error("failed to mark job failed", zap.String("job_id", job.ID.String()), zap.Error(statusErr))
		}
		return fmt.Errorf("get repo for job: %w", err)
	}

	jobID := job.ID.String()
	orgID := repo.ID.String()
	repoIDStr := job.RepoID.String()

	p.log.Info("claimed job",
		zap.String("job_id", jobID),
		zap.String("repo", repo.FullName),
		zap.String("branch", job.Branch),
		zap.String("commit", job.CommitSha),
		zap.Int64("in_flight", p.runner.InFlight()),
	)

	p.runner.GoWait(func() {
		p.executor.Execute(context.Background(), jobID, orgID, repoIDStr)
	})

	return nil
}
