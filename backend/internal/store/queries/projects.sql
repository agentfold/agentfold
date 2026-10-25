-- name: ListProjects :many
SELECT * FROM projects ORDER BY name;

-- name: GetProject :one
SELECT * FROM projects WHERE id = $1;

-- name: InsertProject :one
INSERT INTO projects (name, source_kind, local_path) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateProject :one
UPDATE projects SET name = $2, local_path = $3 WHERE id = $1 RETURNING *;

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = $1;
