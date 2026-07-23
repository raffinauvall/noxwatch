# NoxWatch Agent

The agent is Linux-first, outbound-only, and does not open a listening port. It reads stable Linux interfaces under `/proc`, `/sys`, and mounted filesystem metadata through `statfs`.

## Build

```bash
make agent-build
./dist/noxwatch-agent version
```

The default build uses `CGO_ENABLED=0` and produces a stripped static binary when the target architecture supports it.

## Local Enrollment

1. Generate an enrollment token through `POST /api/v1/servers/enrollment-token`.
2. Create `/etc/noxwatch/agent.yaml` from `agent/packaging/agent.yaml.example`.
3. For a local HTTP API only, set `allow_insecure_http: true`.
4. Write the one-time token to `/etc/noxwatch/enrollment-token` with mode `0600`.
5. Run `make agent-build`, then `sudo make agent-install-local`.

After enrollment the token file is removed. The permanent credential is written atomically to `/etc/noxwatch/credential.json` with mode `0600` and is never printed.

## SSH Bootstrap

For local/LAN installations, choose **SSH bootstrap** in Add Server and enter the SSH username, IPv4/DNS host, port, and an API endpoint reachable from that server. Build the agent once, then run the generated command from the repository root:

```bash
make agent-build
./deployments/scripts/bootstrap-ssh.sh --target deploy@192.168.1.20 \
  --endpoint http://192.168.1.10:8080 \
  --token nox_enroll_example --server-name local-api --environment development
```

OpenSSH prompts for the SSH password locally; the password is never sent to or stored by NoxWatch. The remote user must have `sudo`, the host must use systemd, and the locally built binary must match the remote CPU architecture. Do not use `localhost` as the endpoint unless the NoxWatch API runs on the monitored server or the reverse tunnel below is active.

### Reverse SSH tunnel

During first enrollment, a loopback Agent API endpoint makes the generated bootstrap command create the reverse tunnel automatically:

```bash
./deployments/scripts/bootstrap-ssh.sh --target deploy@203.0.113.10 \
  --endpoint http://127.0.0.1:18082 --reverse-local-port 8082 --reverse-remote-port 18082 \
  --token nox_enroll_example --server-name local-api --environment development
```

The command installs and enrolls the agent, then stays open to keep the tunnel alive. `Open in terminal` generates and runs this command through the local helper.

For later sessions, reconnect an already-enrolled server from the laptop:

```bash
make ssh-tunnel SSH_TARGET=deploy@203.0.113.10
```

`API_PORT` is loaded from `.env`; override `SSH_PORT` or `REMOTE_API_PORT` on the command line when needed. Keep the command running and use `http://127.0.0.1:18082` as the reachable API endpoint in Add Server. The agent connects to server loopback, and OpenSSH carries that traffic to the laptop API over the encrypted session. The remote port is not exposed publicly.

After the first enrollment, start the same tunnel whenever the local NoxWatch stack runs. The agent retries automatically while the tunnel is unavailable; no new enrollment token or agent configuration is required.

The runtime sends heartbeats every 20 seconds and metrics every 45 seconds by default. Failed metric deliveries use exponential backoff and remain in a bounded in-memory queue of 100 samples; oldest samples are dropped when that ceiling is reached.

## Diagnostics

```bash
sudo noxwatch-agent status
noxwatch-agent version
sudo noxwatch-agent config check
sudo noxwatch-agent unregister
```

`unregister` first revokes the credential through the API, then removes the local credential file. Logs are structured JSON and are collected by journald under systemd.
