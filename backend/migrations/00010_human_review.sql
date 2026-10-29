-- +goose Up
-- Human review nodes replace revision runs: a run waits at a human_review
-- node (status awaiting_review) while the human reads its artifacts; a
-- message there sends the node back to the agent, in the same run.
UPDATE node_runs SET status = 'failed', error = 'cancelado', token_hash = NULL, finished_at = now()
WHERE status IN ('queued', 'running', 'awaiting_input', 'waiting')
  AND flow_run_id IN (SELECT id FROM flow_runs WHERE trigger = 'artifact_revision' AND status IN ('running', 'waiting'));
UPDATE flow_runs SET status = 'canceled', error = 'cancelado', finished_at = now()
WHERE trigger = 'artifact_revision' AND status IN ('running', 'waiting');

ALTER TABLE node_runs DROP CONSTRAINT node_runs_status_check;
ALTER TABLE node_runs ADD CONSTRAINT node_runs_status_check
    CHECK (status IN ('queued', 'running', 'awaiting_input', 'waiting', 'awaiting_review', 'done', 'failed'));

-- The human_review node run the artifact was last put under: the review
-- is open while it awaits review.
ALTER TABLE artifacts
    DROP COLUMN frozen_at,
    ADD COLUMN review_node_run_id uuid REFERENCES node_runs (id) ON DELETE SET NULL;

-- Messages belong to the human_review node run they were sent at (human)
-- or that the agent answer reached (agent); older ones to none.
ALTER TABLE artifact_messages
    DROP COLUMN flow_run_id,
    ADD COLUMN node_run_id uuid REFERENCES node_runs (id) ON DELETE SET NULL;
CREATE INDEX artifact_messages_node_run_idx ON artifact_messages (node_run_id);

-- +goose Down
UPDATE node_runs SET status = 'failed', error = 'cancelado', finished_at = now() WHERE status = 'awaiting_review';
ALTER TABLE node_runs DROP CONSTRAINT node_runs_status_check;
ALTER TABLE node_runs ADD CONSTRAINT node_runs_status_check
    CHECK (status IN ('queued', 'running', 'awaiting_input', 'waiting', 'done', 'failed'));
ALTER TABLE artifact_messages
    DROP COLUMN node_run_id,
    ADD COLUMN flow_run_id uuid REFERENCES flow_runs (id) ON DELETE SET NULL;
CREATE INDEX artifact_messages_flow_run_idx ON artifact_messages (flow_run_id);
ALTER TABLE artifacts
    DROP COLUMN review_node_run_id,
    ADD COLUMN frozen_at timestamptz;
