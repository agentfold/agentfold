-- +goose Up
CREATE TABLE flows (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text NOT NULL UNIQUE CHECK (name <> ''),
    enabled    boolean NOT NULL DEFAULT false,
    -- Editable graph; publishing copies it into flow_versions.
    draft      jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE flow_versions (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    flow_id      uuid NOT NULL REFERENCES flows (id) ON DELETE CASCADE,
    version      integer NOT NULL,
    graph        jsonb NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (flow_id, version)
);

-- +goose Down
DROP TABLE flow_versions;
DROP TABLE flows;
