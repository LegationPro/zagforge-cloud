package callback

// Status represents the allowed completion statuses for a job callback.
type Status string

func (s Status) String() string {
	return string(s)
}

const (
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// -- Start --

// StartRequest is the body sent by the worker when it begins processing a job.
type StartRequest struct {
	JobID string `json:"job_id" validate:"required,uuid"`
}

// StartResponse is returned to the worker with clone info for the job.
type StartResponse struct {
	CommitSHA      string `json:"commit_sha"`
	Branch         string `json:"branch"`
	RepoFullName   string `json:"repo_full_name"`
	CloneToken     string `json:"clone_token"`
	InstallationID int64  `json:"installation_id"`
}

// -- Complete --

// CompleteRequest is the body sent by the worker when it finishes processing a job.
type CompleteRequest struct {
	JobID         string `json:"job_id"                   validate:"required,uuid"`
	Status        Status `json:"status"                   validate:"required,oneof=succeeded failed"`
	ErrorMessage  string `json:"error_message,omitempty"  validate:"required_if=Status failed"`
	SnapshotPath  string `json:"snapshot_path,omitempty"  validate:"required_if=Status succeeded"`
	ZigzagVersion string `json:"zigzag_version,omitempty" validate:"required_if=Status succeeded"`
	SizeBytes     int64  `json:"size_bytes,omitempty"     validate:"gte=0"`
	DurationMs    int64  `json:"duration_ms,omitempty"    validate:"gte=0"`
}

// -- Shared --

// StatusResponse is returned after a job state transition.
type StatusResponse struct {
	Status string `json:"status"`
}
