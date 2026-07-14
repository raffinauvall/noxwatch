-- +noxwatch Up
ALTER TABLE alert_events ADD COLUMN notified_at timestamptz;
CREATE INDEX alert_events_rule_notified_idx ON alert_events (alert_rule_id, notified_at DESC) WHERE notified_at IS NOT NULL;

-- +noxwatch Down
DROP INDEX IF EXISTS alert_events_rule_notified_idx;
ALTER TABLE alert_events DROP COLUMN IF EXISTS notified_at;
