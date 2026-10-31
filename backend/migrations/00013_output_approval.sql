-- +goose Up
-- A human approving the output of a node records changes_requested when
-- they ask for changes.
ALTER TABLE node_runs DROP CONSTRAINT node_runs_decision_check,
    ADD CONSTRAINT node_runs_decision_check CHECK (decision IN ('approved', 'rejected', 'changes_requested'));

-- +goose Down
UPDATE node_runs SET decision = NULL WHERE decision = 'changes_requested';
ALTER TABLE node_runs DROP CONSTRAINT node_runs_decision_check,
    ADD CONSTRAINT node_runs_decision_check CHECK (decision IN ('approved', 'rejected'));
