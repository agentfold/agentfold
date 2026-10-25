-- name: InsertJob :one
INSERT INTO jobs (kind, payload, requirements, status, max_attempts)
VALUES ($1, $2, $3, 'queued', $4)
RETURNING *;

-- name: NotifyJobs :exec
SELECT pg_notify('orq_jobs', '');

-- ClaimJob takes the oldest job the worker can run: queued, or running with
-- an expired lease (its worker died) and attempts left.
-- name: ClaimJob :one
UPDATE jobs SET
    status = 'running',
    attempts = attempts + 1,
    locked_by = sqlc.arg(worker),
    lease_until = now() + make_interval(secs => sqlc.arg(lease_seconds)::double precision),
    updated_at = now()
WHERE id = (
    SELECT id FROM jobs
    WHERE run_at <= now()
      AND requirements <@ sqlc.arg(capabilities)::jsonb
      AND (status = 'queued' OR (status = 'running' AND lease_until < now() AND attempts < max_attempts))
    ORDER BY run_at, id
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: ExtendJobLease :execrows
UPDATE jobs
SET lease_until = now() + make_interval(secs => sqlc.arg(lease_seconds)::double precision), updated_at = now()
WHERE id = sqlc.arg(id) AND locked_by = sqlc.arg(worker) AND status = 'running';

-- name: FinishJob :execrows
UPDATE jobs
SET status = sqlc.arg(status), last_error = sqlc.arg(last_error), lease_until = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND locked_by = sqlc.arg(worker) AND status = 'running';
