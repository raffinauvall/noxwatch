#!/bin/sh
set -eu

SCRIPT=./deployments/scripts/reverse-tunnel-ssh.sh

valid() {
	NOXWATCH_TUNNEL_VALIDATE_ONLY=1 "$SCRIPT" --target deploy@192.0.2.10 --port 22 --local-port 8082 --remote-port 18082
	NOXWATCH_TUNNEL_VALIDATE_ONLY=1 "$SCRIPT" --target deploy@server-alias
}

invalid() {
	if NOXWATCH_TUNNEL_VALIDATE_ONLY=1 "$SCRIPT" "$@" 2>/dev/null; then
		printf 'accepted unsafe input: %s\n' "$*" >&2
		exit 1
	fi
}

valid
invalid --target 'deploy@host;touch-pwned'
invalid --target '-oProxyCommand=bad@host'
invalid --target deploy@host --port 70000
invalid --target deploy@host --local-port invalid
invalid --target deploy@host --remote-port 0
printf 'SSH reverse tunnel validation tests passed.\n'
