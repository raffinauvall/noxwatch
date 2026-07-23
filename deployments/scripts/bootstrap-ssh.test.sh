#!/bin/sh
set -eu

SCRIPT=./deployments/scripts/bootstrap-ssh.sh
TOKEN=nox_enroll_12345678901234567890

valid() {
	NOXWATCH_BOOTSTRAP_VALIDATE_ONLY=1 "$SCRIPT" --target deploy@192.168.1.20 --endpoint http://192.168.1.10:8080 --token "$TOKEN" --server-name 'Local API' --environment development --binary /bin/true
	NOXWATCH_BOOTSTRAP_VALIDATE_ONLY=1 "$SCRIPT" --target deploy@192.168.1.20 --endpoint http://127.0.0.1:18082 --token "$TOKEN" --server-name 'Reverse API' --environment development --binary /bin/true --reverse-local-port 8082 --reverse-remote-port 18082
	NOXWATCH_BOOTSTRAP_VALIDATE_ONLY=1 "$SCRIPT" --target deploy@192.168.1.20 --endpoint http://127.0.0.1:18082 --token "$TOKEN" --server-name 'Managed reverse API' --environment development --binary /bin/true --reverse-local-port 8082 --reverse-remote-port 18082 --control-path /tmp/noxwatch-test.sock
}

invalid() {
	if NOXWATCH_BOOTSTRAP_VALIDATE_ONLY=1 "$SCRIPT" "$@" --endpoint http://192.168.1.10:8080 --token "$TOKEN" --server-name server --environment development --binary /bin/true 2>/dev/null; then
		printf 'accepted unsafe input: %s\n' "$*" >&2
		exit 1
	fi
}

valid
invalid --target 'deploy@host;touch-pwned'
invalid --target '-oProxyCommand=bad@host'
invalid --target deploy@host --port 70000
invalid --target deploy@host --reverse-local-port 8082
invalid --target deploy@host --reverse-local-port bad --reverse-remote-port 18082
invalid --target deploy@host --control-path /tmp/noxwatch-test.sock
printf 'SSH bootstrap validation tests passed.\n'
