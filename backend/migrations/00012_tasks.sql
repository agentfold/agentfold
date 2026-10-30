-- +goose Up
-- Card types name the lane the tasks agents create of them start in; a
-- task the agent canceled no longer counts for WaitChildren.
ALTER TABLE card_types ADD COLUMN initial_lane_id uuid REFERENCES lanes (id) ON DELETE SET NULL;
ALTER TABLE cards ADD COLUMN canceled_at timestamptz;

-- +goose Down
ALTER TABLE cards DROP COLUMN canceled_at;
ALTER TABLE card_types DROP COLUMN initial_lane_id;
