-- +goose Up
-- What an agent did while running a node, summarized from its stream-json
-- output for the card page (the raw stream stays in the card logs).
CREATE TABLE node_run_output (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    node_run_id uuid NOT NULL REFERENCES node_runs (id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN ('text', 'tool_use', 'tool_result', 'result')),
    -- text: the message, or the tool input/result, shortened.
    text        text NOT NULL,
    -- tool_use: the tool name.
    tool        text,
    is_error    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX node_run_output_node_run_idx ON node_run_output (node_run_id, id);

-- +goose Down
DROP TABLE node_run_output;
