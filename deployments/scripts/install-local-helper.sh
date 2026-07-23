#!/bin/sh
set -eu

fail() {
	printf 'local helper install: %s\n' "$*" >&2
	exit 1
}

usage() {
	printf '%s\n' 'usage: install-local-helper.sh --repo-root PATH --origin URL --addr HOST:PORT --local-api-port PORT'
	exit 2
}

REPO_ROOT=
ORIGIN=
ADDR=
LOCAL_API_PORT=

while [ "$#" -gt 0 ]; do
	case "$1" in
		--repo-root|--origin|--addr|--local-api-port)
			[ "$#" -ge 2 ] || usage
			case "$1" in
				--repo-root) REPO_ROOT=$2 ;;
				--origin) ORIGIN=$2 ;;
				--addr) ADDR=$2 ;;
				--local-api-port) LOCAL_API_PORT=$2 ;;
			esac
			shift 2
			;;
		*) usage ;;
	esac
done

[ -n "$REPO_ROOT" ] && [ -n "$ORIGIN" ] && [ -n "$ADDR" ] && [ -n "$LOCAL_API_PORT" ] || usage
case "$REPO_ROOT" in /*) ;; *) fail 'repository path must be absolute' ;; esac
case "$REPO_ROOT$ORIGIN$ADDR" in *[[:space:]\"\']*) fail 'paths and addresses must not contain whitespace or quotes' ;; esac
case "$LOCAL_API_PORT" in ''|*[!0-9]*) fail 'API port must be numeric' ;; esac
[ "$LOCAL_API_PORT" -ge 1 ] && [ "$LOCAL_API_PORT" -le 65535 ] || fail 'API port must be between 1 and 65535'
[ -x "$REPO_ROOT/dist/noxwatch-local-helper" ] || fail 'build dist/noxwatch-local-helper first'

BIN_DIR=${XDG_BIN_HOME:-"$HOME/.local/bin"}
UNIT_DIR=${XDG_CONFIG_HOME:-"$HOME/.config"}/systemd/user
install -d -m 0755 "$BIN_DIR" "$UNIT_DIR"
install -m 0755 "$REPO_ROOT/dist/noxwatch-local-helper" "$BIN_DIR/noxwatch-local-helper"
printf '[Unit]\nDescription=NoxWatch local tunnel helper\nAfter=graphical-session.target\n\n[Service]\nExecStart=%s -repo-root %s -origin %s -addr %s -local-api-port %s\nRestart=on-failure\n\n[Install]\nWantedBy=default.target\n' \
	"$BIN_DIR/noxwatch-local-helper" "$REPO_ROOT" "$ORIGIN" "$ADDR" "$LOCAL_API_PORT" >"$UNIT_DIR/noxwatch-local-helper.service"
systemctl --user daemon-reload
systemctl --user enable noxwatch-local-helper.service
systemctl --user restart noxwatch-local-helper.service
printf 'NoxWatch local helper installed and started. It will start automatically after login.\n'
