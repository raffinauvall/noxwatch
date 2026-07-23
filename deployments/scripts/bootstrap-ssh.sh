#!/bin/sh
set -eu

fail() {
	printf 'bootstrap: %s\n' "$*" >&2
	exit 1
}

usage() {
	printf '%s\n' 'usage: bootstrap-ssh.sh --target user@host --endpoint URL --token TOKEN --server-name NAME --environment ENV [--port 22] [--binary PATH] [--reverse-local-port PORT --reverse-remote-port PORT]'
	exit 2
}

TARGET=
ENDPOINT=
TOKEN=
SERVER_NAME=
ENVIRONMENT=
PORT=22
BINARY=./dist/noxwatch-agent
SERVICE=./deployments/systemd/noxwatch-agent.service
REVERSE_LOCAL_PORT=
REVERSE_REMOTE_PORT=

while [ "$#" -gt 0 ]; do
	case "$1" in
		--target|--endpoint|--token|--server-name|--environment|--port|--binary|--service|--reverse-local-port|--reverse-remote-port)
			[ "$#" -ge 2 ] || usage
			case "$1" in
				--target) TARGET=$2 ;;
				--endpoint) ENDPOINT=$2 ;;
				--token) TOKEN=$2 ;;
				--server-name) SERVER_NAME=$2 ;;
				--environment) ENVIRONMENT=$2 ;;
				--port) PORT=$2 ;;
				--binary) BINARY=$2 ;;
				--service) SERVICE=$2 ;;
				--reverse-local-port) REVERSE_LOCAL_PORT=$2 ;;
				--reverse-remote-port) REVERSE_REMOTE_PORT=$2 ;;
			esac
			shift 2
			;;
		-h|--help) usage ;;
		*) usage ;;
	esac
done

case "$TARGET" in *@*) ;; *) fail 'target must use user@host' ;; esac
SSH_USER=${TARGET%%@*}
SSH_HOST=${TARGET#*@}
case "$SSH_USER" in ''|-*|*[!A-Za-z0-9._-]*) fail 'SSH username contains unsupported characters' ;; esac
case "$SSH_HOST" in ''|-*|*[!A-Za-z0-9.-]*) fail 'SSH host must be an IPv4 address or DNS name' ;; esac
case "$PORT" in ''|*[!0-9]*) fail 'SSH port must be numeric' ;; esac
[ "$PORT" -ge 1 ] && [ "$PORT" -le 65535 ] || fail 'SSH port must be between 1 and 65535'
if [ -n "$REVERSE_LOCAL_PORT" ] || [ -n "$REVERSE_REMOTE_PORT" ]; then
	[ -n "$REVERSE_LOCAL_PORT" ] && [ -n "$REVERSE_REMOTE_PORT" ] || fail 'both reverse tunnel ports are required'
	for VALUE in "$REVERSE_LOCAL_PORT" "$REVERSE_REMOTE_PORT"; do
		case "$VALUE" in ''|*[!0-9]*) fail 'reverse tunnel ports must be numeric' ;; esac
		[ "$VALUE" -ge 1 ] && [ "$VALUE" -le 65535 ] || fail 'reverse tunnel ports must be between 1 and 65535'
	done
fi
case "$ENDPOINT" in
	https://*) ALLOW_INSECURE=false ;;
	http://*) ALLOW_INSECURE=true ;;
	*) fail 'endpoint must use HTTP or HTTPS' ;;
esac
case "$ENDPOINT" in *"@"*|*"'"*|*[[:space:]]*|*"
"*) fail 'endpoint contains unsupported characters' ;; esac
case "$SERVER_NAME" in ''|*"'"*|*"
"*) fail 'server name is empty or contains unsupported characters' ;; esac
case "$ENVIRONMENT" in production|staging|development|testing|other) ;; *) fail 'environment is invalid' ;; esac
case "$TOKEN" in nox_enroll_*) ;; *) fail 'enrollment token is invalid' ;; esac
[ "${#TOKEN}" -ge 31 ] || fail 'enrollment token is invalid'
case "$TOKEN" in *[!A-Za-z0-9_-]*) fail 'enrollment token is invalid' ;; esac
[ -x "$BINARY" ] || fail "agent binary is not executable: $BINARY"
[ -f "$SERVICE" ] || fail "systemd unit not found: $SERVICE"

[ "${NOXWATCH_BOOTSTRAP_VALIDATE_ONLY:-}" = 1 ] && exit 0
command -v ssh >/dev/null 2>&1 || fail 'ssh is required'
command -v scp >/dev/null 2>&1 || fail 'scp is required'

umask 077
TMP=$(mktemp -d)
CONTROL=$TMP/ssh-control
REMOTE_DIR=
cleanup() {
	if [ -n "$REMOTE_DIR" ]; then
		ssh -S "$CONTROL" -p "$PORT" "$TARGET" "rm -rf '$REMOTE_DIR'" >/dev/null 2>&1 || true
	fi
	ssh -S "$CONTROL" -O exit "$TARGET" >/dev/null 2>&1 || true
	rm -rf "$TMP"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

cp "$BINARY" "$TMP/noxwatch-agent"
cp "$SERVICE" "$TMP/noxwatch-agent.service"
printf '%s' "$TOKEN" >"$TMP/enrollment-token"
printf "endpoint: '%s'\nserver_name: '%s'\nenvironment: %s\nenrollment_file: /etc/noxwatch/enrollment-token\ncredential_file: /etc/noxwatch/credential.json\nallow_insecure_http: %s\n" \
	"$ENDPOINT" "$SERVER_NAME" "$ENVIRONMENT" "$ALLOW_INSECURE" >"$TMP/agent.yaml"

printf 'Connecting to %s. Enter the SSH password when prompted.\n' "$TARGET"
if [ -n "$REVERSE_LOCAL_PORT" ]; then
	ssh -M -S "$CONTROL" -o ControlPersist=60 -o ExitOnForwardFailure=yes -fN -p "$PORT" \
		-R "127.0.0.1:$REVERSE_REMOTE_PORT:127.0.0.1:$REVERSE_LOCAL_PORT" "$TARGET"
else
	ssh -M -S "$CONTROL" -o ControlPersist=60 -fN -p "$PORT" "$TARGET"
fi
REMOTE_DIR=$(ssh -S "$CONTROL" -p "$PORT" "$TARGET" 'mktemp -d /tmp/noxwatch-bootstrap.XXXXXX')
case "$REMOTE_DIR" in /tmp/noxwatch-bootstrap.*) ;; *) fail 'remote temporary directory is invalid' ;; esac

scp -P "$PORT" -o "ControlPath=$CONTROL" "$TMP/noxwatch-agent" "$TMP/noxwatch-agent.service" "$TMP/agent.yaml" "$TMP/enrollment-token" "$TARGET:$REMOTE_DIR/"
ssh -t -S "$CONTROL" -p "$PORT" "$TARGET" "set -eu; trap 'rm -rf \"$REMOTE_DIR\"' EXIT; test \"\$(uname -s)\" = Linux; command -v systemctl >/dev/null; sudo install -d -m 0700 /etc/noxwatch; sudo install -m 0755 '$REMOTE_DIR/noxwatch-agent' /usr/local/bin/noxwatch-agent; sudo install -m 0600 '$REMOTE_DIR/agent.yaml' /etc/noxwatch/agent.yaml; sudo install -m 0600 '$REMOTE_DIR/enrollment-token' /etc/noxwatch/enrollment-token; sudo install -m 0644 '$REMOTE_DIR/noxwatch-agent.service' /etc/systemd/system/noxwatch-agent.service; sudo systemctl daemon-reload; sudo systemctl enable --now noxwatch-agent; sudo systemctl is-active --quiet noxwatch-agent"

REMOTE_DIR=
printf 'NoxWatch agent installed on %s.\n' "$TARGET"
if [ -n "$REVERSE_LOCAL_PORT" ]; then
	printf 'Reverse tunnel is active: server localhost:%s -> local API localhost:%s. Press Ctrl+C to stop.\n' "$REVERSE_REMOTE_PORT" "$REVERSE_LOCAL_PORT"
	while ssh -S "$CONTROL" -O check -p "$PORT" "$TARGET" >/dev/null 2>&1; do
		sleep 5
	done
	fail 'reverse tunnel disconnected'
fi
