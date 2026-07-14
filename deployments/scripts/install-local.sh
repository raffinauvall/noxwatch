#!/bin/sh
set -eu

BINARY=${1:-./dist/noxwatch-agent}
CONFIG=${2:-./agent/packaging/agent.yaml.example}

test "$(id -u)" -eq 0 || { echo "run as root" >&2; exit 1; }
test -x "$BINARY" || { echo "agent binary not found: $BINARY" >&2; exit 1; }
test -f "$CONFIG" || { echo "agent config not found: $CONFIG" >&2; exit 1; }

install -d -m 0700 /etc/noxwatch
install -m 0755 "$BINARY" /usr/local/bin/noxwatch-agent
install -m 0600 "$CONFIG" /etc/noxwatch/agent.yaml
install -m 0644 ./deployments/systemd/noxwatch-agent.service /etc/systemd/system/noxwatch-agent.service
systemctl daemon-reload
systemctl enable --now noxwatch-agent
