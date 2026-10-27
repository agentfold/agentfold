-- +goose Up
-- WaitChildren nodes wait for the child cards in a node run status of their own.
ALTER TABLE node_runs DROP CONSTRAINT node_runs_status_check;
ALTER TABLE node_runs ADD CONSTRAINT node_runs_status_check
    CHECK (status IN ('queued', 'running', 'awaiting_input', 'waiting', 'done', 'failed'));

-- +goose Down
UPDATE node_runs SET status = 'failed', error = 'cancelado', finished_at = now() WHERE status = 'waiting';
ALTER TABLE node_runs DROP CONSTRAINT node_runs_status_check;
ALTER TABLE node_runs ADD CONSTRAINT node_runs_status_check
    CHECK (status IN ('queued', 'running', 'awaiting_input', 'done', 'failed'));
