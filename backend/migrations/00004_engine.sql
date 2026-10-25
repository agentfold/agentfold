-- +goose Up
-- Existing runs predate the engine; nothing has created any yet.
ALTER TABLE flow_runs
    ADD COLUMN flow_id uuid NOT NULL REFERENCES flows (id) ON DELETE RESTRICT,
    ADD COLUMN error text;

-- A flow runs at most once at a time for a card.
CREATE UNIQUE INDEX flow_runs_active_idx ON flow_runs (card_id, flow_id) WHERE status IN ('running', 'waiting');

-- Set when the engine followed the output of the node run, so a retried job
-- does not follow it twice.
ALTER TABLE node_runs ADD COLUMN continued_at timestamptz;

-- +goose Down
ALTER TABLE node_runs DROP COLUMN continued_at;
DROP INDEX flow_runs_active_idx;
ALTER TABLE flow_runs DROP COLUMN error, DROP COLUMN flow_id;
