package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	github "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/runner"
	"go.uber.org/zap"
)

// mockCloner is a test double for runner.RepoCloner.
type mockCloner struct {
	token    string
	tokenErr error
	cloneErr error
}

func (m *mockCloner) GenerateCloneToken(_ context.Context, _ int64) (string, error) {
	return m.token, m.tokenErr
}

func (m *mockCloner) CloneRepo(_ context.Context, _, _, _, dst string) error {
	if m.cloneErr != nil {
		return m.cloneErr
	}
	// Create the destination directory so the runner can proceed.
	return os.MkdirAll(dst, 0o755)
}

// mockZigzag writes a shell script that produces a report.json in the --output-dir.
func mockZigzag(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "zigzag")
	// The script parses --output-dir from args and writes report.json there.
	script := `#!/bin/sh
OUTDIR=""
while [ $# -gt 0 ]; do
  case "$1" in
    --output-dir) OUTDIR="$2"; shift 2;;
    *) shift;;
  esac
done
if [ -n "$OUTDIR" ]; then
  mkdir -p "$OUTDIR"
  cat > "$OUTDIR/report.json" <<'ENDJSON'
{"meta":{"version":"0.15.1"}}
ENDJSON
fi
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write mock zigzag: %v", err)
	}
	return path
}

func newRunner(t *testing.T, cloner *mockCloner) (*runner.Runner, runner.Config) {
	t.Helper()
	cfg := runner.Config{
		WorkspaceDir: t.TempDir(),
		ZigzagBin:    mockZigzag(t),
		ReportsDir:   t.TempDir(),
	}
	return runner.New(cloner, cfg, zap.NewNop()), cfg
}

func TestRun_success(t *testing.T) {
	cloner := &mockCloner{token: "ghs_test"}
	r, _ := newRunner(t, cloner)

	event := github.WebhookEvent{
		RepoName:       "org/repo",
		CloneURL:       "https://github.com/org/repo.git",
		Branch:         "main",
		CommitSHA:      "abc123",
		InstallationID: 42,
	}

	result, err := r.Run(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ZigzagVersion != "0.15.1" {
		t.Errorf("expected zigzag version %q, got %q", "0.15.1", result.ZigzagVersion)
	}
	if result.SizeBytes <= 0 {
		t.Errorf("expected positive size, got %d", result.SizeBytes)
	}
}

func TestRun_success_reportsDir_containsReportJSON(t *testing.T) {
	cloner := &mockCloner{token: "ghs_test"}
	r, cfg := newRunner(t, cloner)

	result, err := r.Run(context.Background(), github.WebhookEvent{InstallationID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify report.json exists in the reports dir.
	reportPath := filepath.Join(cfg.ReportsDir, "report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report.json not found: %v", err)
	}

	var report struct {
		Meta struct {
			Version string `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid report.json: %v", err)
	}
	if report.Meta.Version != result.ZigzagVersion {
		t.Errorf("version mismatch: report.json=%q result=%q", report.Meta.Version, result.ZigzagVersion)
	}
}

func TestRun_tokenError(t *testing.T) {
	cloner := &mockCloner{tokenErr: errors.New("auth failed")}
	r, _ := newRunner(t, cloner)

	_, err := r.Run(context.Background(), github.WebhookEvent{})
	if err == nil {
		t.Fatal("expected error from token failure, got nil")
	}
}

func TestRun_cloneError(t *testing.T) {
	cloner := &mockCloner{token: "ghs_test", cloneErr: errors.New("clone failed")}
	r, _ := newRunner(t, cloner)

	_, err := r.Run(context.Background(), github.WebhookEvent{InstallationID: 1})
	if err == nil {
		t.Fatal("expected error from clone failure, got nil")
	}
}

func TestRun_zigzagError(t *testing.T) {
	cloner := &mockCloner{token: "ghs_test"}

	// Write a zigzag that exits non-zero.
	dir := t.TempDir()
	failBin := filepath.Join(dir, "zigzag")
	os.WriteFile(failBin, []byte("#!/bin/sh\nexit 1\n"), 0o755)

	r := runner.New(cloner, runner.Config{
		WorkspaceDir: t.TempDir(),
		ZigzagBin:    failBin,
		ReportsDir:   t.TempDir(),
	}, zap.NewNop())

	_, err := r.Run(context.Background(), github.WebhookEvent{InstallationID: 1})
	if err == nil {
		t.Fatal("expected error from zigzag failure, got nil")
	}
}

// -- InFlight / Drain tests --

func TestInFlight_tracksRunningJobs(t *testing.T) {
	cloner := &mockCloner{token: "ghs_test"}
	r, _ := newRunner(t, cloner)

	if n := r.InFlight(); n != 0 {
		t.Fatalf("expected 0 in-flight, got %d", n)
	}

	started := make(chan struct{})
	release := make(chan struct{})

	r.GoWait(func() {
		close(started)
		<-release
	})

	<-started
	if n := r.InFlight(); n != 1 {
		t.Fatalf("expected 1 in-flight, got %d", n)
	}

	close(release)
	r.Wait()

	if n := r.InFlight(); n != 0 {
		t.Fatalf("expected 0 in-flight after wait, got %d", n)
	}
}

func TestDrain_completesWhenJobsFinish(t *testing.T) {
	cloner := &mockCloner{token: "ghs_test"}
	r, _ := newRunner(t, cloner)

	release := make(chan struct{})
	r.GoWait(func() {
		<-release
	})

	// Release the job after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(release)
	}()

	err := r.Drain(5*time.Second, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("expected drain to succeed, got: %v", err)
	}
}

func TestDrain_timesOutWithRemainingJobs(t *testing.T) {
	cloner := &mockCloner{token: "ghs_test"}
	r, _ := newRunner(t, cloner)

	release := make(chan struct{})
	r.GoWait(func() {
		<-release
	})
	defer close(release) // cleanup

	err := r.Drain(100*time.Millisecond, 25*time.Millisecond)
	if err == nil {
		t.Fatal("expected drain timeout error, got nil")
	}
}

func TestDrain_noJobs_returnsImmediately(t *testing.T) {
	cloner := &mockCloner{token: "ghs_test"}
	r, _ := newRunner(t, cloner)

	start := time.Now()
	err := r.Drain(5*time.Second, 1*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected fast return, took %s", elapsed)
	}
}

func TestInFlight_multipleJobs(t *testing.T) {
	cloner := &mockCloner{token: "ghs_test"}
	r, _ := newRunner(t, cloner)

	const n = 5
	var started sync.WaitGroup
	release := make(chan struct{})

	started.Add(n)
	for range n {
		r.GoWait(func() {
			started.Done()
			<-release
		})
	}

	started.Wait()
	if got := r.InFlight(); got != n {
		t.Fatalf("expected %d in-flight, got %d", n, got)
	}

	close(release)
	r.Wait()

	if got := r.InFlight(); got != 0 {
		t.Fatalf("expected 0 in-flight after wait, got %d", got)
	}
}
