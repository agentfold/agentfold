-- +goose Up
-- Gateways replace review nodes: an agent node with the gateway on decides
-- with the approve or reject tools. Reviews had no data worth keeping.
DROP TABLE review_comments;
ALTER TABLE node_runs
    DROP COLUMN review_approved,
    ADD COLUMN decision text CHECK (decision IN ('approved', 'rejected')),
    -- The reason of approve or the feedback of reject.
    ADD COLUMN feedback text;

-- Files of the card workspace an agent node collects when it finishes, by
-- name. Moving the card to another lane freezes them; collecting again
-- (a later node declaring the same name) opens them again.
CREATE TABLE artifacts (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    card_id     uuid NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    name        text NOT NULL CHECK (name <> ''),
    -- The node run that collected the latest version: revisions resume
    -- its agent session.
    node_run_id uuid NOT NULL REFERENCES node_runs (id) ON DELETE CASCADE,
    frozen_at   timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (card_id, name)
);

CREATE TABLE artifact_versions (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    artifact_id uuid NOT NULL REFERENCES artifacts (id) ON DELETE CASCADE,
    version     integer NOT NULL,
    path        text NOT NULL,
    content     text NOT NULL,
    node_run_id uuid NOT NULL REFERENCES node_runs (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, version)
);

-- The review chat of an artifact. Each human message starts a revision run
-- (flow_run_id); the agent answer closes it.
CREATE TABLE artifact_messages (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    artifact_id uuid NOT NULL REFERENCES artifacts (id) ON DELETE CASCADE,
    -- The version the human was reading, or the agent revised.
    version     integer NOT NULL,
    role        text NOT NULL CHECK (role IN ('human', 'agent')),
    text        text NOT NULL CHECK (text <> ''),
    flow_run_id uuid REFERENCES flow_runs (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX artifact_messages_artifact_idx ON artifact_messages (artifact_id, created_at);
CREATE INDEX artifact_messages_flow_run_idx ON artifact_messages (flow_run_id);

-- +goose Down
DROP TABLE artifact_messages;
DROP TABLE artifact_versions;
DROP TABLE artifacts;
ALTER TABLE node_runs
    DROP COLUMN feedback,
    DROP COLUMN decision,
    ADD COLUMN review_approved boolean;
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
