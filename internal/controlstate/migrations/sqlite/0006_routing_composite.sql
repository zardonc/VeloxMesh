-- +goose Up
ALTER TABLE routing_configs ADD COLUMN composite_json TEXT;

-- +goose Down
ALTER TABLE routing_configs DROP COLUMN composite_json;
