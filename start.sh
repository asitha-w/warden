#!/usr/bin/env sh
# Warden, batteries included: run this and open http://localhost:8080.
# Finds or fetches a Go toolchain, builds the console, starts it. No other
# dependencies. Set ANTHROPIC_API_KEY for live mode; without it the agents
# run from scripted playbooks.
set -eu
cd "$(dirname "$0")"
NEED="1.27"
TOOLS=".tools"
GO=""
if command -v go >/dev/null 2>&1; then
  have=$(go version | sed -E 's/.*go([0-9]+\.[0-9]+).*/\1/')
  if [ "$(printf '%s\n%s\n' "$NEED" "$have" | sort -V | head -1)" = "$NEED" ]; then GO=go; fi
fi
if [ -z "$GO" ] && [ -x "$TOOLS/go/bin/go" ]; then GO="$TOOLS/go/bin/go"; fi
if [ -z "$GO" ]; then
  os=$(uname -s | tr '[:upper:]' '[:lower:]'); arch=$(uname -m)
  case "$arch" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac
  ver=$(curl -fsSL "https://go.dev/VERSION?m=text" | head -1)
  echo "Go $NEED or newer not found; fetching $ver into $TOOLS/ (project-local, nothing installed system-wide)"
  mkdir -p "$TOOLS" && curl -fsSL "https://go.dev/dl/$ver.$os-$arch.tar.gz" | tar -xz -C "$TOOLS"
  GO="$TOOLS/go/bin/go"
fi
echo "Using $($GO version)"
mkdir -p bin
$GO build -o bin/warden ./cmd/warden
ADDR="${WARDEN_ADDR:-:8080}"
MODE="scripted mode (set ANTHROPIC_API_KEY for live agents)"
[ -n "${ANTHROPIC_API_KEY:-}" ] && MODE="live mode, model ${WARDEN_MODEL:-claude-opus-5-5}"
echo "Warden on http://localhost${ADDR} · $MODE · Ctrl+C to stop"
if [ "${1:-}" = "--dev" ]; then export WARDEN_WEB_DIR=web; echo "dev mode: console served from web/"; fi
exec env WARDEN_ADDR="$ADDR" ./bin/warden
