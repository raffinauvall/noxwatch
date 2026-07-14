-- +noxwatch Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL,
  email_normalized text GENERATED ALWAYS AS (lower(email)) STORED,
  password_hash text NOT NULL,
  name text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (email_normalized)
);

CREATE TABLE workspaces (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  slug text NOT NULL UNIQUE,
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workspace_members (
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id)
);

CREATE TABLE sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  refresh_token_hash text NOT NULL UNIQUE,
  user_agent text NOT NULL DEFAULT '',
  ip_address inet,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE servers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name text NOT NULL,
  hostname text NOT NULL DEFAULT '',
  description text NOT NULL DEFAULT '',
  environment text NOT NULL DEFAULT 'production' CHECK (environment IN ('production', 'staging', 'development', 'testing', 'other')),
  operating_system text NOT NULL DEFAULT '',
  os_version text NOT NULL DEFAULT '',
  kernel_version text NOT NULL DEFAULT '',
  architecture text NOT NULL DEFAULT '',
  primary_ip inet,
  public_ip inet,
  agent_version text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'unknown' CHECK (status IN ('online', 'degraded', 'warning', 'offline', 'unknown', 'maintenance')),
  last_seen_at timestamptz,
  enrolled_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX servers_workspace_status_idx ON servers (workspace_id, status);
CREATE INDEX servers_last_seen_idx ON servers (last_seen_at);

CREATE TABLE agents (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  server_id uuid NOT NULL UNIQUE REFERENCES servers(id) ON DELETE CASCADE,
  credential_hash text NOT NULL,
  credential_version integer NOT NULL DEFAULT 1,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE enrollment_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  server_name text NOT NULL,
  environment text NOT NULL DEFAULT 'production' CHECK (environment IN ('production', 'staging', 'development', 'testing', 'other')),
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  revoked_at timestamptz,
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX enrollment_tokens_workspace_idx ON enrollment_tokens (workspace_id, expires_at);

CREATE TABLE server_tags (
  server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  tag text NOT NULL,
  PRIMARY KEY (server_id, tag)
);

CREATE TABLE metric_samples (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  agent_id uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  sequence bigint NOT NULL,
  collected_at timestamptz NOT NULL,
  uptime_seconds bigint NOT NULL DEFAULT 0,
  process_count integer NOT NULL DEFAULT 0,
  zombie_process_count integer,
  logged_in_users_count integer,
  cpu_usage_percent numeric(5,2),
  load_1 numeric(8,2),
  load_5 numeric(8,2),
  load_15 numeric(8,2),
  logical_cpu_count integer,
  physical_cpu_count integer,
  memory_total_bytes bigint,
  memory_used_bytes bigint,
  memory_available_bytes bigint,
  memory_usage_percent numeric(5,2),
  swap_total_bytes bigint,
  swap_used_bytes bigint,
  swap_usage_percent numeric(5,2),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (agent_id, sequence)
);

CREATE INDEX metric_samples_server_time_idx ON metric_samples (server_id, collected_at DESC);
CREATE INDEX metric_samples_workspace_time_idx ON metric_samples (workspace_id, collected_at DESC);

CREATE TABLE disk_metric_samples (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  metric_sample_id uuid NOT NULL REFERENCES metric_samples(id) ON DELETE CASCADE,
  mount_point text NOT NULL,
  filesystem text NOT NULL,
  total_bytes bigint NOT NULL,
  used_bytes bigint NOT NULL,
  available_bytes bigint,
  usage_percent numeric(5,2),
  inode_usage_percent numeric(5,2)
);

CREATE TABLE network_metric_samples (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  metric_sample_id uuid NOT NULL REFERENCES metric_samples(id) ON DELETE CASCADE,
  interface_name text NOT NULL,
  rx_bytes_total bigint NOT NULL,
  tx_bytes_total bigint NOT NULL,
  rx_packets_total bigint,
  tx_packets_total bigint,
  rx_errors_total bigint,
  tx_errors_total bigint,
  rx_bytes_per_second numeric(14,2),
  tx_bytes_per_second numeric(14,2)
);

CREATE TABLE alert_rules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  server_id uuid REFERENCES servers(id) ON DELETE CASCADE,
  name text NOT NULL,
  metric text NOT NULL,
  warning_threshold numeric(10,2),
  critical_threshold numeric(10,2),
  evaluation_seconds integer NOT NULL DEFAULT 300,
  cooldown_seconds integer NOT NULL DEFAULT 900,
  enabled boolean NOT NULL DEFAULT true,
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX alert_rules_workspace_idx ON alert_rules (workspace_id, enabled);

CREATE TABLE alert_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  alert_rule_id uuid NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
  server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  severity text NOT NULL CHECK (severity IN ('warning', 'critical')),
  state text NOT NULL CHECK (state IN ('pending', 'firing', 'resolved', 'acknowledged')),
  current_value numeric(10,2),
  threshold numeric(10,2),
  triggered_at timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz,
  acknowledged_at timestamptz
);

CREATE INDEX alert_events_workspace_state_idx ON alert_events (workspace_id, state, triggered_at DESC);

CREATE TABLE notification_channels (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  type text NOT NULL CHECK (type IN ('webhook', 'telegram', 'email', 'discord', 'slack')),
  name text NOT NULL,
  config jsonb NOT NULL DEFAULT '{}'::jsonb,
  secret_hash text,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid REFERENCES workspaces(id) ON DELETE SET NULL,
  actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  actor_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
  action text NOT NULL,
  target_type text NOT NULL,
  target_id uuid,
  ip_address inet,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_workspace_time_idx ON audit_logs (workspace_id, created_at DESC);

-- +noxwatch Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS alert_events;
DROP TABLE IF EXISTS alert_rules;
DROP TABLE IF EXISTS network_metric_samples;
DROP TABLE IF EXISTS disk_metric_samples;
DROP TABLE IF EXISTS metric_samples;
DROP TABLE IF EXISTS server_tags;
DROP TABLE IF EXISTS enrollment_tokens;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS servers;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS workspace_members;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS users;
