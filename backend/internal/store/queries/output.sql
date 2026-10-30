-- name: InsertNodeOutput :one
INSERT INTO node_run_output (node_run_id, kind, text, tool, is_error) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListNodeOutput :many
SELECT * FROM node_run_output WHERE node_run_id = $1 ORDER BY id;

-- name: ListCardFlowRuns :many
SELECT sqlc.embed(r), f.name AS flow_name,
    EXISTS (SELECT 1 FROM node_runs n WHERE n.flow_run_id = r.id AND n.status = 'awaiting_review') AS awaiting_review
FROM flow_runs r JOIN flows f ON f.id = r.flow_id
WHERE r.card_id = $1
ORDER BY r.started_at DESC, r.id DESC;

-- name: CancelFlowRun :one
UPDATE flow_runs SET status = 'canceled', error = 'cancelado', finished_at = now()
WHERE id = $1 AND status IN ('running', 'waiting')
RETURNING *;

-- name: FailOpenNodeRuns :exec
UPDATE node_runs SET status = 'failed', error = 'cancelado', token_hash = NULL, finished_at = now()
WHERE flow_run_id = $1 AND status IN ('queued', 'running', 'awaiting_input', 'waiting', 'awaiting_review');

-- name: CloseRunQuestions :exec
UPDATE questions SET delivered_at = now()
WHERE delivered_at IS NULL AND node_run_id IN (SELECT id FROM node_runs WHERE flow_run_id = $1);

-- CancelRunJobs stops the agent jobs of a run: a worker running one loses
-- its lease at the next heartbeat and interrupts the agent.
-- name: CancelRunJobs :exec
UPDATE jobs SET status = 'failed', last_error = 'canceled', lease_until = NULL, updated_at = now()
WHERE kind = 'agent.run' AND status IN ('queued', 'running')
  AND (payload->>'node_run_id')::uuid IN (SELECT id FROM node_runs WHERE flow_run_id = $1);
