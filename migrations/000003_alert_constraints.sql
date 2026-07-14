-- +noxwatch Up
ALTER TABLE alert_rules
  ADD CONSTRAINT alert_rules_metric_check CHECK (metric IN ('cpu_usage','memory_usage','disk_usage','swap_usage','server_offline','agent_disconnected')),
  ADD CONSTRAINT alert_rules_duration_check CHECK (evaluation_seconds >= 0 AND cooldown_seconds >= 0),
  ADD CONSTRAINT alert_rules_threshold_check CHECK (
    (metric IN ('server_offline','agent_disconnected')) OR
    (warning_threshold BETWEEN 0 AND 100 AND critical_threshold BETWEEN 0 AND 100 AND critical_threshold >= warning_threshold)
  );

CREATE UNIQUE INDEX alert_events_one_open_idx
  ON alert_events (alert_rule_id, server_id)
  WHERE state IN ('pending','firing','acknowledged');

-- +noxwatch Down
DROP INDEX IF EXISTS alert_events_one_open_idx;
ALTER TABLE alert_rules
  DROP CONSTRAINT IF EXISTS alert_rules_threshold_check,
  DROP CONSTRAINT IF EXISTS alert_rules_duration_check,
  DROP CONSTRAINT IF EXISTS alert_rules_metric_check;
