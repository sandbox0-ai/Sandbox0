-- +goose Up
ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS home_region_id TEXT;

CREATE INDEX IF NOT EXISTS idx_teams_home_region_id ON teams(home_region_id);

-- +goose Down
DROP INDEX IF EXISTS idx_teams_home_region_id;

ALTER TABLE teams
    DROP COLUMN IF EXISTS home_region_id;
