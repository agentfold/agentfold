-- name: ListCardTypes :many
SELECT * FROM card_types ORDER BY name;

-- name: GetCardType :one
SELECT * FROM card_types WHERE id = $1;

-- name: InsertCardType :one
INSERT INTO card_types (name, color, initial_lane_id) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateCardType :one
UPDATE card_types SET name = $2, color = $3, initial_lane_id = $4 WHERE id = $1 RETURNING *;

-- name: DeleteCardType :execrows
DELETE FROM card_types WHERE id = $1;
