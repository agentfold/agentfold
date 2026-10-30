-- name: ListCards :many
SELECT * FROM cards ORDER BY lane_id, position, created_at;

-- name: ListChildCards :many
SELECT * FROM cards WHERE parent_card_id = $1 ORDER BY created_at;

-- name: GetCard :one
SELECT * FROM cards WHERE id = $1;

-- name: GetCardForUpdate :one
SELECT * FROM cards WHERE id = $1 FOR UPDATE;

-- name: CountCardsInLane :one
SELECT count(*)::integer FROM cards WHERE lane_id = $1;

-- name: InsertCard :one
INSERT INTO cards (title, description, card_type_id, project_id, parent_card_id, lane_id, position, depends_on)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateCard :one
UPDATE cards
SET title = $2, description = $3, card_type_id = $4, project_id = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: PlaceCard :one
UPDATE cards SET lane_id = $2, position = $3, updated_at = now() WHERE id = $1 RETURNING *;

-- name: ShiftCards :exec
UPDATE cards SET position = position + sqlc.arg(delta)::integer
WHERE lane_id = sqlc.arg(lane_id)
  AND position >= sqlc.arg(from_pos)::integer AND position <= sqlc.arg(to_pos)::integer;

-- name: SetCardProperty :one
UPDATE cards SET properties = properties || jsonb_build_object(sqlc.arg(key)::text, sqlc.arg(value)::text), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListCardsByID :many
SELECT * FROM cards WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY created_at, id;

-- name: ListDependentCards :many
SELECT * FROM cards WHERE sqlc.arg(id)::uuid = ANY(depends_on) ORDER BY created_at, id;

-- SetCardCanceled marks a child card canceled, or (canceled false)
-- not anymore.
-- name: SetCardCanceled :one
UPDATE cards SET canceled_at = CASE WHEN sqlc.arg(canceled)::boolean THEN now() END, updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteCard :execrows
DELETE FROM cards WHERE id = $1;

-- name: InsertCardEvent :one
INSERT INTO card_events (card_id, type, payload) VALUES ($1, $2, $3) RETURNING *;

-- name: ListCardEvents :many
SELECT * FROM card_events WHERE card_id = $1 ORDER BY created_at, id;

-- name: GetCardEvent :one
SELECT * FROM card_events WHERE id = $1;
