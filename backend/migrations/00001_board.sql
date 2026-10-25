-- +goose Up
CREATE TABLE lanes (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text NOT NULL CHECK (name <> ''),
    position   integer NOT NULL,
    color      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    name        text NOT NULL UNIQUE CHECK (name <> ''),
    source_kind text NOT NULL CHECK (source_kind IN ('local')),
    local_path  text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE card_types (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text NOT NULL UNIQUE CHECK (name <> ''),
    color      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cards (
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    title          text NOT NULL CHECK (title <> ''),
    description    text NOT NULL DEFAULT '',
    card_type_id   uuid NOT NULL REFERENCES card_types (id) ON DELETE RESTRICT,
    project_id     uuid REFERENCES projects (id) ON DELETE RESTRICT,
    parent_card_id uuid REFERENCES cards (id) ON DELETE RESTRICT,
    lane_id        uuid NOT NULL REFERENCES lanes (id) ON DELETE RESTRICT,
    position       integer NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cards_lane_position_idx ON cards (lane_id, position);
CREATE INDEX cards_parent_idx ON cards (parent_card_id);

CREATE TABLE card_events (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    card_id    uuid NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    type       text NOT NULL,
    payload    jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX card_events_card_idx ON card_events (card_id, created_at);

-- +goose Down
DROP TABLE card_events;
DROP TABLE cards;
DROP TABLE card_types;
DROP TABLE projects;
DROP TABLE lanes;
