#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

if [[ ! -f .env ]]; then
  cat >&2 <<'EOF'
[run.sh] Missing .env in current directory.
Create one first, for example:

AUTH_MODE=token
GATEWAY_TOKEN=claw_2026_demo_001
LISTEN_ADDR=:8099
DEFAULT_AGENT=main
OPENCLAW_BIN=openclaw
DATA_DIR=./data
ENABLE_STREAMING=true
EOF
  exit 1
fi

set -a
# shellcheck disable=SC1091
source ./.env
set +a

mkdir -p "${DATA_DIR:-./data}"

exec go run .
