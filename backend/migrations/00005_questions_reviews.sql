-- +goose Up
-- What the agent reports through the board MCP tools while its node runs.
-- token_hash authenticates those calls (sha256 of the token handed to the run).
ALTER TABLE node_runs
    ADD COLUMN token_hash bytea UNIQUE,
    ADD COLUMN summary text,
    ADD COLUMN structured_output jsonb,
    ADD COLUMN review_approved boolean;

-- Questions an agent asked a human. The node waits until every question it
-- asked is answered; delivered_at is set when the answers resume it.
CREATE TABLE questions (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    card_id      uuid NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    node_run_id  uuid NOT NULL REFERENCES node_runs (id) ON DELETE CASCADE,
    text         text NOT NULL CHECK (text <> ''),
    answer       text,
    answered_at  timestamptz,
    delivered_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX questions_card_idx ON questions (card_id);
CREATE INDEX questions_node_run_idx ON questions (node_run_id);

-- Comments of a review round (the iteration of the review node run).
CREATE TABLE review_comments (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    card_id     uuid NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    node_run_id uuid NOT NULL REFERENCES node_runs (id) ON DELETE CASCADE,
    round       integer NOT NULL,
    file        text,
    line        integer,
    body        text NOT NULL CHECK (body <> ''),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX review_comments_node_run_idx ON review_comments (node_run_id);

-- +goose Down
DROP TABLE review_comments;
DROP TABLE questions;
ALTER TABLE node_runs
    DROP COLUMN review_approved,
    DROP COLUMN structured_output,
    DROP COLUMN summary,
    DROP COLUMN token_hash;
