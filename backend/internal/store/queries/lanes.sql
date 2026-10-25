-- name: ListLanes :many
SELECT * FROM lanes ORDER BY position, created_at;

-- name: GetLane :one
SELECT * FROM lanes WHERE id = $1;

-- name: GetLaneForUpdate :one
SELECT * FROM lanes WHERE id = $1 FOR UPDATE;

-- name: CountLanes :one
SELECT count(*)::integer FROM lanes;

-- name: InsertLane :one
INSERT INTO lanes (name, color, position) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateLane :one
UPDATE lanes SET name = $2, color = $3 WHERE id = $1 RETURNING *;

-- name: SetLanePosition :one
UPDATE lanes SET position = $2 WHERE id = $1 RETURNING *;

-- name: ShiftLanes :exec
UPDATE lanes SET position = position + sqlc.arg(delta)::integer
WHERE position >= sqlc.arg(from_pos)::integer AND position <= sqlc.arg(to_pos)::integer;

-- name: DeleteLane :execrows
DELETE FROM lanes WHERE id = $1;
