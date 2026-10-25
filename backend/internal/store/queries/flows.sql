-- name: ListFlows :many
SELECT f.*, coalesce((SELECT max(v.version) FROM flow_versions v WHERE v.flow_id = f.id), 0)::integer AS latest_version
FROM flows f
ORDER BY f.name;

-- name: GetFlow :one
SELECT f.*, coalesce((SELECT max(v.version) FROM flow_versions v WHERE v.flow_id = f.id), 0)::integer AS latest_version
FROM flows f
WHERE f.id = $1;

-- name: GetFlowForUpdate :one
SELECT * FROM flows WHERE id = $1 FOR UPDATE;

-- name: InsertFlow :one
INSERT INTO flows (name, draft) VALUES ($1, $2) RETURNING *;

-- name: UpdateFlow :one
UPDATE flows SET name = $2, enabled = $3, updated_at = now() WHERE id = $1 RETURNING *;

-- name: UpdateFlowDraft :one
UPDATE flows SET draft = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteFlow :execrows
DELETE FROM flows WHERE id = $1;

-- name: NextFlowVersion :one
SELECT (coalesce(max(version), 0) + 1)::integer FROM flow_versions WHERE flow_id = $1;

-- name: InsertFlowVersion :one
INSERT INTO flow_versions (flow_id, version, graph) VALUES ($1, $2, $3) RETURNING *;

-- name: ListFlowVersions :many
SELECT * FROM flow_versions WHERE flow_id = $1 ORDER BY version DESC;
