-- +goose Up
-- A Split node run opens parallel branches of its flow run; settled_at
-- marks when its Join went on (or its branches all ended before one).
CREATE TABLE flow_splits (
    node_run_id uuid PRIMARY KEY REFERENCES node_runs (id) ON DELETE CASCADE,
    flow_run_id uuid NOT NULL REFERENCES flow_runs (id) ON DELETE CASCADE,
    settled_at  timestamptz
);

-- A branch of a split, nested in the branch the split ran in (parent).
-- label names it in the run: split.a, or split.a/inner.b when nested.
CREATE TABLE flow_branches (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    flow_run_id       uuid NOT NULL REFERENCES flow_runs (id) ON DELETE CASCADE,
    split_node_run_id uuid NOT NULL REFERENCES flow_splits (node_run_id) ON DELETE CASCADE,
    split_key         text NOT NULL,
    parent_branch_id  uuid REFERENCES flow_branches (id) ON DELETE CASCADE,
    name              text NOT NULL,
    label             text NOT NULL,
    status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'arrived', 'done', 'failed', 'canceled')),
    output            jsonb,
    error             text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX flow_branches_split_idx ON flow_branches (split_node_run_id);
CREATE INDEX flow_branches_parent_idx ON flow_branches (parent_branch_id);

-- The branch a node run ran in; none for the main path of the run.
ALTER TABLE node_runs ADD COLUMN branch_id uuid REFERENCES flow_branches (id) ON DELETE CASCADE,
    ADD COLUMN branch text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE node_runs DROP COLUMN branch, DROP COLUMN branch_id;
DROP TABLE flow_branches;
DROP TABLE flow_splits;
