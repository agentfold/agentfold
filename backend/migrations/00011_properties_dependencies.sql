-- +goose Up
-- Cards carry properties (key and value) that flow nodes set and check,
-- and the cards they depend on: a card that others depend on cannot be
-- deleted, which the board checks (arrays have no foreign keys).
ALTER TABLE cards
    ADD COLUMN properties jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN depends_on uuid[] NOT NULL DEFAULT '{}';
CREATE INDEX cards_depends_on_idx ON cards USING gin (depends_on);

-- +goose Down
ALTER TABLE cards DROP COLUMN properties, DROP COLUMN depends_on;
