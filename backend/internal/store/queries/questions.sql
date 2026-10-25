-- name: InsertQuestion :one
INSERT INTO questions (card_id, node_run_id, text) VALUES ($1, $2, $3) RETURNING *;

-- name: GetQuestion :one
SELECT * FROM questions WHERE id = $1;

-- name: ListCardQuestions :many
SELECT * FROM questions WHERE card_id = $1 ORDER BY created_at, id;

-- name: AnswerQuestion :one
UPDATE questions SET answer = $2, answered_at = now() WHERE id = $1 AND delivered_at IS NULL RETURNING *;

-- name: ListUndeliveredQuestions :many
SELECT * FROM questions WHERE node_run_id = $1 AND delivered_at IS NULL ORDER BY created_at, id;

-- name: DeliverQuestions :exec
UPDATE questions SET delivered_at = now() WHERE node_run_id = $1 AND delivered_at IS NULL;

-- name: DeleteReviewComments :exec
DELETE FROM review_comments WHERE node_run_id = $1;

-- name: InsertReviewComment :one
INSERT INTO review_comments (card_id, node_run_id, round, file, line, body) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: ListReviewComments :many
SELECT * FROM review_comments WHERE node_run_id = $1 ORDER BY created_at, id;
