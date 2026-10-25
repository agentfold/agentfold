-- +goose Up
CREATE TABLE agent_profiles (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    kind            text NOT NULL CHECK (kind IN ('claude')),
    name            text NOT NULL UNIQUE CHECK (name <> ''),
    -- Sealed with ORQ_SECRET_KEY; set by `orquestrator agent login`.
    credential      bytea,
    credential_kind text CHECK (credential_kind IN ('oauth_token', 'api_key')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK ((credential IS NULL) = (credential_kind IS NULL))
);

CREATE TABLE mcp_servers (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text NOT NULL UNIQUE CHECK (name <> ''),
    transport  text NOT NULL CHECK (transport IN ('stdio', 'http')),
    -- stdio: command, args, env; http: url, headers. Secret values live in secrets.
    config     jsonb NOT NULL,
    -- Sealed JSON object of secret env vars (stdio) or headers (http).
    secrets    bytea,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE flow_runs (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    flow_version_id uuid NOT NULL REFERENCES flow_versions (id) ON DELETE RESTRICT,
    card_id         uuid NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    trigger         text NOT NULL,
    status          text NOT NULL CHECK (status IN ('running', 'waiting', 'done', 'failed', 'canceled')),
    started_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz
);

CREATE INDEX flow_runs_card_idx ON flow_runs (card_id);

CREATE TABLE node_runs (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    flow_run_id   uuid NOT NULL REFERENCES flow_runs (id) ON DELETE CASCADE,
    node_key      text NOT NULL,
    -- How many times the node was entered in the run (review loops re-enter agents).
    iteration     integer NOT NULL DEFAULT 1,
    status        text NOT NULL CHECK (status IN ('queued', 'running', 'awaiting_input', 'done', 'failed')),
    input         jsonb NOT NULL DEFAULT '{}',
    output        jsonb,
    -- Output (edge source handle) the run left through.
    output_handle text,
    session_id    uuid,
    tokens_in     bigint NOT NULL DEFAULT 0,
    tokens_out    bigint NOT NULL DEFAULT 0,
    cache_read    bigint NOT NULL DEFAULT 0,
    cache_write   bigint NOT NULL DEFAULT 0,
    cost_usd      double precision NOT NULL DEFAULT 0,
    turns         integer NOT NULL DEFAULT 0,
    duration_ms   bigint NOT NULL DEFAULT 0,
    error         text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    started_at    timestamptz,
    finished_at   timestamptz
);

CREATE INDEX node_runs_flow_run_idx ON node_runs (flow_run_id);

-- One agent session per agent node of a run, resumed when the node is re-entered.
CREATE TABLE agent_sessions (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    flow_run_id       uuid NOT NULL REFERENCES flow_runs (id) ON DELETE CASCADE,
    node_key          text NOT NULL,
    profile_id        uuid NOT NULL REFERENCES agent_profiles (id) ON DELETE RESTRICT,
    claude_session_id uuid NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (flow_run_id, node_key)
);

CREATE TABLE jobs (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    kind         text NOT NULL,
    payload      jsonb NOT NULL DEFAULT '{}',
    -- Must be contained in the capabilities a worker announces, e.g. {"host": "x"}.
    requirements jsonb NOT NULL DEFAULT '{}',
    status       text NOT NULL CHECK (status IN ('queued', 'running', 'done', 'failed')),
    run_at       timestamptz NOT NULL DEFAULT now(),
    attempts     integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 2,
    locked_by    text,
    lease_until  timestamptz,
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX jobs_pending_idx ON jobs (run_at) WHERE status IN ('queued', 'running');

-- +goose Down
DROP TABLE jobs;
DROP TABLE agent_sessions;
DROP TABLE node_runs;
DROP TABLE flow_runs;
DROP TABLE mcp_servers;
DROP TABLE agent_profiles;
