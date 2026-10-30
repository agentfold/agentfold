-- InsertFlowRun starts a run unless the flow is already running for the card.
-- name: InsertFlowRun :one
INSERT INTO flow_runs (flow_id, flow_version_id, card_id, trigger, status)
VALUES ($1, $2, $3, $4, 'running')
ON CONFLICT (card_id, flow_id) WHERE status IN ('running', 'waiting') DO NOTHING
RETURNING *;

-- name: GetFlowRun :one
SELECT sqlc.embed(r), f.name AS flow_name,
    EXISTS (SELECT 1 FROM node_runs n WHERE n.flow_run_id = r.id AND n.status = 'awaiting_review') AS awaiting_review
FROM flow_runs r JOIN flows f ON f.id = r.flow_id
WHERE r.id = $1;

-- ListLatestFlowRuns returns one run per card: an active one if there is
-- any, else the last one.
-- name: ListLatestFlowRuns :many
SELECT DISTINCT ON (r.card_id) sqlc.embed(r), f.name AS flow_name,
    EXISTS (SELECT 1 FROM node_runs n WHERE n.flow_run_id = r.id AND n.status = 'awaiting_review') AS awaiting_review
FROM flow_runs r JOIN flows f ON f.id = r.flow_id
ORDER BY r.card_id, r.status IN ('running', 'waiting') DESC, r.started_at DESC, r.id DESC;

-- name: FinishFlowRun :one
UPDATE flow_runs SET status = $2, error = $3, finished_at = now()
WHERE id = $1 AND status IN ('running', 'waiting')
RETURNING *;

-- name: CardHasActiveRun :one
SELECT EXISTS (SELECT 1 FROM flow_runs WHERE card_id = $1 AND status IN ('running', 'waiting'));

-- name: ListRunnableFlows :many
SELECT DISTINCT ON (v.flow_id) sqlc.embed(v), f.name AS flow_name
FROM flow_versions v JOIN flows f ON f.id = v.flow_id
WHERE f.enabled
ORDER BY v.flow_id, v.version DESC;

-- name: InsertNodeRun :one
INSERT INTO node_runs (flow_run_id, node_key, iteration, status, input) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetNodeRun :one
SELECT * FROM node_runs WHERE id = $1;

-- name: CountNodeRuns :one
SELECT count(*)::integer FROM node_runs WHERE flow_run_id = $1 AND node_key = $2;

-- name: StartNodeRun :one
UPDATE node_runs SET status = 'running', started_at = now(), finished_at = NULL, session_id = $2, token_hash = $3
WHERE id = $1 RETURNING *;

-- FinishNodeRun adds the usage: a node resumed after questions runs again.
-- name: FinishNodeRun :one
UPDATE node_runs
SET status = $2, output = $3, output_handle = $4, error = $5,
    tokens_in = tokens_in + $6, tokens_out = tokens_out + $7,
    cache_read = cache_read + $8, cache_write = cache_write + $9,
    cost_usd = cost_usd + $10, turns = turns + $11, duration_ms = duration_ms + $12,
    token_hash = NULL, finished_at = now()
WHERE id = $1
RETURNING *;

-- name: ListNodeRuns :many
SELECT * FROM node_runs WHERE flow_run_id = $1 ORDER BY created_at, id;

-- name: GetRunningNodeRunByToken :one
SELECT * FROM node_runs WHERE token_hash = $1 AND status = 'running';

-- name: SetNodeRunSummary :one
UPDATE node_runs SET summary = $2 WHERE id = $1 RETURNING *;

-- name: SetNodeRunOutput :one
UPDATE node_runs SET structured_output = $2 WHERE id = $1 RETURNING *;

-- name: SetNodeRunDecision :one
UPDATE node_runs SET decision = $2, feedback = $3 WHERE id = $1 RETURNING *;

-- name: GetNodeRunForUpdate :one
SELECT * FROM node_runs WHERE id = $1 FOR UPDATE;

-- name: QueueNodeRun :exec
UPDATE node_runs SET status = 'queued' WHERE id = $1;

-- name: SetFlowRunStatus :one
UPDATE flow_runs SET status = sqlc.arg(status) WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status) RETURNING *;

-- ContinueNodeRun claims the right to follow the output of a finished node run.
-- name: ContinueNodeRun :one
UPDATE node_runs SET continued_at = now()
WHERE id = $1 AND continued_at IS NULL AND status IN ('done', 'failed')
RETURNING *;

-- name: GetAgentSession :one
SELECT * FROM agent_sessions WHERE flow_run_id = $1 AND node_key = $2;

-- name: InsertAgentSession :one
INSERT INTO agent_sessions (flow_run_id, node_key, profile_id, claude_session_id) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: WaitNodeRun :one
UPDATE node_runs SET status = 'waiting' WHERE id = $1 AND status IN ('queued', 'running') RETURNING *;

-- name: AwaitReviewNodeRun :one
UPDATE node_runs SET status = 'awaiting_review' WHERE id = $1 AND status IN ('queued', 'running') RETURNING *;

-- name: GetNodeRunByIteration :one
SELECT * FROM node_runs WHERE flow_run_id = $1 AND node_key = $2 AND iteration = $3;

-- WaitingNodeRun finds the node run (WaitChildren or CheckProperty) a
-- card's flow is waiting at, if any.
-- name: WaitingNodeRun :one
SELECT n.* FROM node_runs n JOIN flow_runs r ON r.id = n.flow_run_id
WHERE r.card_id = $1 AND r.status = 'waiting' AND n.status = 'waiting'
ORDER BY n.created_at DESC LIMIT 1;
