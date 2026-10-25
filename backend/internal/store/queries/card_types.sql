-- name: ListCardTypes :many
SELECT * FROM card_types ORDER BY name;

-- name: GetCardType :one
SELECT * FROM card_types WHERE id = $1;

-- name: InsertCardType :one
INSERT INTO card_types (name, color) VALUES ($1, $2) RETURNING *;

-- name: UpdateCardType :one
UPDATE card_types SET name = $2, color = $3 WHERE id = $1 RETURNING *;

-- name: DeleteCardType :execrows
DELETE FROM card_types WHERE id = $1;
