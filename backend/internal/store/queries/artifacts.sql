-- name: UpsertArtifact :one
INSERT INTO artifacts (card_id, name, node_run_id) VALUES ($1, $2, $3)
ON CONFLICT (card_id, name) DO UPDATE SET node_run_id = EXCLUDED.node_run_id, frozen_at = NULL, updated_at = now()
RETURNING *;

-- name: LatestArtifactVersion :one
SELECT * FROM artifact_versions WHERE artifact_id = $1 ORDER BY version DESC LIMIT 1;

-- name: InsertArtifactVersion :one
INSERT INTO artifact_versions (artifact_id, version, path, content, node_run_id) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetArtifact :one
SELECT * FROM artifacts WHERE id = $1;

-- name: GetArtifactForUpdate :one
SELECT * FROM artifacts WHERE id = $1 FOR UPDATE;

-- name: ListCardArtifacts :many
SELECT * FROM artifacts WHERE card_id = $1 ORDER BY created_at, id;

-- name: ListArtifactVersions :many
SELECT * FROM artifact_versions WHERE artifact_id = $1 ORDER BY version;

-- name: ListArtifactMessages :many
SELECT * FROM artifact_messages WHERE artifact_id = $1 ORDER BY created_at, id;

-- name: InsertArtifactMessage :one
INSERT INTO artifact_messages (artifact_id, version, role, text, flow_run_id) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetRevisionMessage :one
SELECT * FROM artifact_messages WHERE flow_run_id = $1 AND role = 'human';

-- FreezeCardArtifacts freezes the artifacts of a card that left its lane,
-- except those a run still active collected: that run moves the card on.
-- name: FreezeCardArtifacts :many
UPDATE artifacts a SET frozen_at = now(), updated_at = now()
WHERE a.card_id = $1 AND a.frozen_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM node_runs n JOIN flow_runs r ON r.id = n.flow_run_id
    WHERE n.id = a.node_run_id AND r.status IN ('running', 'waiting'))
RETURNING *;

-- name: ArtifactRevising :one
SELECT EXISTS (
  SELECT 1 FROM artifact_messages m JOIN flow_runs r ON r.id = m.flow_run_id
  WHERE m.artifact_id = $1 AND r.status IN ('running', 'waiting'));
