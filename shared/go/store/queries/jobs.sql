-- name: CreateJob :one
INSERT INTO jobs (repo_id, branch, commit_sha, delivery_id)
VALUES ($1, $2, $3, NULLIF($4, ''))
RETURNING *;

-- name: GetActiveJobsForBranch :many
SELECT * FROM jobs
WHERE repo_id = $1
  AND branch = $2
  AND status IN ('queued', 'running')
ORDER BY created_at ASC;

-- name: MarkJobSuperseded :exec
UPDATE jobs
SET status = 'superseded'
WHERE id = $1
  AND status = 'queued';

-- name: GetJobByID :one
SELECT * FROM jobs WHERE id = $1;

-- name: ListJobsByRepo :many
SELECT * FROM jobs
WHERE repo_id = $1
  AND created_at < $2
ORDER BY created_at DESC
LIMIT $3;

-- name: GetRepoForJob :one
SELECT r.id, r.github_repo_id, r.installation_id, r.full_name, r.default_branch
FROM repositories r
JOIN jobs j ON j.repo_id = r.id
WHERE j.id = $1;

-- name: ClaimJob :one
UPDATE jobs
SET status = 'running', started_at = now()
WHERE id = (
    SELECT id FROM jobs
    WHERE status = 'queued'
    ORDER BY created_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: UpdateJobStatus :exec
UPDATE jobs
SET status = $2,
    error_message = $3,
    started_at    = CASE WHEN $2 = 'running' THEN now() ELSE started_at END,
    finished_at   = CASE WHEN $2 IN ('succeeded', 'failed', 'cancelled', 'superseded') THEN now() ELSE finished_at END
WHERE id = $1;

-- name: GetJobForUpdate :one
SELECT * FROM jobs WHERE id = $1 FOR UPDATE;
