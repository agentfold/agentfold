-- name: InsertFlowRun :one
INSERT INTO flow_runs (flow_version_id, card_id, trigger, status) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetFlowRun :one
SELECT * FROM flow_runs WHERE id = $1;

-- name: InsertNodeRun :one
INSERT INTO node_runs (flow_run_id, node_key, iteration, status, input) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetNodeRun :one
SELECT * FROM node_runs WHERE id = $1;

-- name: StartNodeRun :one
UPDATE node_runs SET status = 'running', started_at = now(), session_id = $2 WHERE id = $1 RETURNING *;

-- name: FinishNodeRun :one
UPDATE node_runs
SET status = $2, output = $3, output_handle = $4, error = $5,
    tokens_in = $6, tokens_out = $7, cache_read = $8, cache_write = $9,
    cost_usd = $10, turns = $11, duration_ms = $12, finished_at = now()
WHERE id = $1
RETURNING *;

-- name: GetAgentSession :one
SELECT * FROM agent_sessions WHERE flow_run_id = $1 AND node_key = $2;

-- name: InsertAgentSession :one
INSERT INTO agent_sessions (flow_run_id, node_key, profile_id, claude_session_id) VALUES ($1, $2, $3, $4) RETURNING *;
