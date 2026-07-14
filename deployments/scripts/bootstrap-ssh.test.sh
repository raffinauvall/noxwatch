#!/bin/sh
set -eu

SCRIPT=./deployments/scripts/bootstrap-ssh.sh
TOKEN=nox_enroll_12345678901234567890

valid() {
	NOXWATCH_BOOTSTRAP_VALIDATE_ONLY=1 "$SCRIPT" --target deploy@192.168.1.20 --endpoint http://192.168.1.10:8080 --token "$TOKEN" --server-name 'Local API' --environment development --binary /bin/true
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
printf 'SSH bootstrap validation tests passed.\n'
