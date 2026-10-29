-- name: UpsertArtifact :one
INSERT INTO artifacts (card_id, name, node_run_id) VALUES ($1, $2, $3)
ON CONFLICT (card_id, name) DO UPDATE SET node_run_id = EXCLUDED.node_run_id, updated_at = now()
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
INSERT INTO artifact_messages (artifact_id, version, role, text, node_run_id) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- OpenArtifactReview puts artifacts of the card under a human_review node
-- run: those named, or with no names those collected in its flow run.
-- name: OpenArtifactReview :many
UPDATE artifacts a SET review_node_run_id = @node_run_id, updated_at = now()
WHERE a.card_id = @card_id AND (
    a.name = ANY(@names::text[])
    OR (cardinality(@names::text[]) = 0
        AND a.node_run_id IN (SELECT n.id FROM node_runs n WHERE n.flow_run_id = @flow_run_id)))
RETURNING *;

-- ReviewRequest is the human message that sent a human_review node run
-- back to the agent.
-- name: ReviewRequest :one
SELECT * FROM artifact_messages WHERE node_run_id = $1 AND role = 'human'
ORDER BY created_at DESC, id DESC LIMIT 1;

-- ArtifactReview is the state of the human_review node run the artifact
-- is under, and of its flow run.
-- name: ArtifactReview :one
SELECT n.status, n.output_handle, r.status AS run_status
FROM artifacts a
JOIN node_runs n ON n.id = a.review_node_run_id
JOIN flow_runs r ON r.id = n.flow_run_id
WHERE a.id = $1;
