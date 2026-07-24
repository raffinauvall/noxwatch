-- +noxwatch Up
CREATE TABLE server_tunnel_profiles (
  server_id uuid PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
  ssh_user text NOT NULL,
  ssh_host text NOT NULL,
  ssh_port integer NOT NULL CHECK (ssh_port BETWEEN 1 AND 65535),
  remote_port integer NOT NULL DEFAULT 18082 CHECK (remote_port BETWEEN 1 AND 65535),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- +noxwatch Down
DROP TABLE IF EXISTS server_tunnel_profiles;
