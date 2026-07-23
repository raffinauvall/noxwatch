#!/bin/sh
set -eu

fail() {
	printf 'reverse tunnel: %s\n' "$*" >&2
	exit 1
}

usage() {
	printf '%s\n' 'usage: reverse-tunnel-ssh.sh --target user@host [--port PORT] [--local-port 8080] [--remote-port 18082]'
	exit 2
}

TARGET=
PORT=
LOCAL_PORT=8080
REMOTE_PORT=18082

while [ "$#" -gt 0 ]; do
	case "$1" in
		--target|--port|--local-port|--remote-port)
			[ "$#" -ge 2 ] || usage
			case "$1" in
				--target) TARGET=$2 ;;
				--port) PORT=$2 ;;
				--local-port) LOCAL_PORT=$2 ;;
				--remote-port) REMOTE_PORT=$2 ;;
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

for VALUE in "$LOCAL_PORT" "$REMOTE_PORT"; do
	case "$VALUE" in ''|*[!0-9]*) fail 'ports must be numeric' ;; esac
	[ "$VALUE" -ge 1 ] && [ "$VALUE" -le 65535 ] || fail 'ports must be between 1 and 65535'
done
if [ -n "$PORT" ]; then
	case "$PORT" in *[!0-9]*) fail 'ports must be numeric' ;; esac
	[ "$PORT" -ge 1 ] && [ "$PORT" -le 65535 ] || fail 'ports must be between 1 and 65535'
fi

[ "${NOXWATCH_TUNNEL_VALIDATE_ONLY:-}" = 1 ] && exit 0
command -v ssh >/dev/null 2>&1 || fail 'ssh is required'

printf 'Forwarding %s localhost:%s to NoxWatch API localhost:%s. Press Ctrl+C to stop.\n' "$TARGET" "$REMOTE_PORT" "$LOCAL_PORT"
set -- -NT \
	-o ExitOnForwardFailure=yes \
	-o ServerAliveInterval=30 \
	-o ServerAliveCountMax=3 \
	-R "127.0.0.1:$REMOTE_PORT:127.0.0.1:$LOCAL_PORT" \
	"$TARGET"
[ -z "$PORT" ] || set -- -p "$PORT" "$@"
exec ssh "$@"
