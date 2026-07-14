-- +noxwatch Up
ALTER TABLE enrollment_tokens
  ADD COLUMN description text NOT NULL DEFAULT '',
  ADD COLUMN tags text[] NOT NULL DEFAULT '{}',
  ADD COLUMN server_id uuid REFERENCES servers(id) ON DELETE SET NULL;

ALTER TABLE agents
  ADD COLUMN machine_id_hash text NOT NULL DEFAULT '';

CREATE INDEX enrollment_tokens_hash_active_idx
  ON enrollment_tokens (token_hash, expires_at)
  WHERE used_at IS NULL AND revoked_at IS NULL;

-- +noxwatch Down
DROP INDEX IF EXISTS enrollment_tokens_hash_active_idx;
ALTER TABLE agents DROP COLUMN IF EXISTS machine_id_hash;
ALTER TABLE enrollment_tokens
  DROP COLUMN IF EXISTS server_id,
  DROP COLUMN IF EXISTS tags,
  DROP COLUMN IF EXISTS description;
