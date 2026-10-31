-- name: InsertFlowSplit :exec
INSERT INTO flow_splits (node_run_id, flow_run_id) VALUES ($1, $2);

-- name: GetFlowSplitForUpdate :one
SELECT * FROM flow_splits WHERE node_run_id = $1 FOR UPDATE;

-- name: SettleFlowSplit :exec
UPDATE flow_splits SET settled_at = now() WHERE node_run_id = $1;

-- name: InsertFlowBranch :one
INSERT INTO flow_branches (flow_run_id, split_node_run_id, split_key, parent_branch_id, name, label)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetFlowBranch :one
SELECT * FROM flow_branches WHERE id = $1;

-- name: ListSplitBranches :many
SELECT * FROM flow_branches WHERE split_node_run_id = $1 ORDER BY created_at, id;

-- name: EndFlowBranch :execrows
UPDATE flow_branches SET status = $2, output = $3, error = $4, updated_at = now()
WHERE id = $1 AND status = 'active';

-- name: CancelSplitBranches :many
-- Cancels the active branches of a split and those nested in them.
WITH RECURSIVE doomed AS (
    SELECT fb.id FROM flow_branches fb WHERE fb.split_node_run_id = $1 AND fb.status = 'active'
    UNION
    SELECT b.id FROM flow_branches b JOIN doomed d ON b.parent_branch_id = d.id WHERE b.status = 'active'
)
UPDATE flow_branches SET status = 'canceled', updated_at = now()
WHERE id IN (SELECT doomed.id FROM doomed) RETURNING id;

-- name: FailBranchNodeRuns :exec
UPDATE node_runs SET status = 'failed', error = 'ramo cancelado', token_hash = NULL, finished_at = now()
WHERE branch_id = ANY(@branch_ids::uuid[]) AND status IN ('queued', 'running', 'awaiting_input', 'waiting', 'awaiting_review');
