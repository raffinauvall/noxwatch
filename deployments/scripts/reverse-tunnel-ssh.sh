#!/bin/sh
set -eu

fail() {
	printf 'reverse tunnel: %s\n' "$*" >&2
	exit 1
}

usage() {
	printf '%s\n' 'usage: reverse-tunnel-ssh.sh --target user@host [--port PORT] [--local-port 8080] [--remote-port 18082] [--background --control-path PATH]'
	exit 2
}

TARGET=
PORT=
LOCAL_PORT=8080
REMOTE_PORT=18082
BACKGROUND=false
CONTROL_PATH=

while [ "$#" -gt 0 ]; do
	case "$1" in
	--target|--port|--local-port|--remote-port|--control-path)
			[ "$#" -ge 2 ] || usage
			case "$1" in
				--target) TARGET=$2 ;;
				--port) PORT=$2 ;;
				--local-port) LOCAL_PORT=$2 ;;
				--remote-port) REMOTE_PORT=$2 ;;
				--control-path) CONTROL_PATH=$2 ;;
			esac
			shift 2
			;;
		--background)
			BACKGROUND=true
			shift
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
if [ "$BACKGROUND" = true ]; then
	[ -n "$CONTROL_PATH" ] || fail 'background mode requires a control path'
	case "$CONTROL_PATH" in /*) ;; *) fail 'control path must be absolute' ;; esac
fi

[ "${NOXWATCH_TUNNEL_VALIDATE_ONLY:-}" = 1 ] && exit 0
command -v ssh >/dev/null 2>&1 || fail 'ssh is required'

if [ "$BACKGROUND" = true ] && ssh -S "$CONTROL_PATH" -O check -p "${PORT:-22}" "$TARGET" >/dev/null 2>&1; then
	printf 'Tunnel already active for %s.\n' "$TARGET"
	exit 0
fi

printf 'Forwarding %s localhost:%s to NoxWatch API localhost:%s.\n' "$TARGET" "$REMOTE_PORT" "$LOCAL_PORT"
set -- -NT \
	-o ExitOnForwardFailure=yes \
	-o ServerAliveInterval=30 \
	-o ServerAliveCountMax=3 \
	-R "127.0.0.1:$REMOTE_PORT:127.0.0.1:$LOCAL_PORT" \
	"$TARGET"
[ -z "$PORT" ] || set -- -p "$PORT" "$@"
if [ "$BACKGROUND" = true ]; then
	rm -f -- "$CONTROL_PATH"
	set -- -fM -S "$CONTROL_PATH" "$@"
	ssh "$@"
	printf 'Tunnel started in background for %s.\n' "$TARGET"
	exit 0
fi
printf 'Press Ctrl+C to stop.\n'
exec ssh "$@"
