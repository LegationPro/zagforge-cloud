package callback

// Status represents the allowed completion statuses for a job callback.
type Status string

func (s Status) IsValid() bool {
	switch s {
	case StatusSucceeded, StatusFailed:
		return true
	default:
		return false
	}
}

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
	JobID string `json:"job_id"`
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
	JobID         string `json:"job_id"`
	Status        Status `json:"status"`
	ErrorMessage  string `json:"error_message,omitempty"`
	SnapshotPath  string `json:"snapshot_path,omitempty"`
	ZigzagVersion string `json:"zigzag_version,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
}

// -- Shared --

// StatusResponse is returned after a job state transition.
type StatusResponse struct {
	Status string `json:"status"`
}
