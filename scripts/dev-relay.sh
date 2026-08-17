#!/usr/bin/env bash
# dev 三件套:relay-server + daemon(cloud 指向本机 relay)
# 用法:  scripts/dev-relay.sh
# 结束:  Ctrl+C(daemon)后自动停 relay
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> starting relay-server on 127.0.0.1:8788"
(cd "$root/server" && go run ./cmd/relay-server) &
relay_pid=$!
sleep 3

cleanup() { kill "$relay_pid" 2>/dev/null || true; }
trap cleanup EXIT

echo "==> starting daemon (data: \${TMPDIR:-/tmp}/ssd-relay-dev)"
export SHELLSYNC_DATA_DIR="${TMPDIR:-/tmp}/ssd-relay-dev"
cd "$root/daemon"
go run ./cmd/shellsync-daemon
