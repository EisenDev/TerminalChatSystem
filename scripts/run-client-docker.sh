#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

: "${CHAT_SERVER_URL:=http://localhost:18080}"
: "${CHAT_WORKSPACE:=acme}"
: "${CHAT_HANDLE:=}"

exec docker run --rm -it --network host \
  -v "$ROOT_DIR:/app" -w /app \
  -e "CHAT_SERVER_URL=$CHAT_SERVER_URL" \
  -e "CHAT_WORKSPACE=$CHAT_WORKSPACE" \
  -e "CHAT_HANDLE=$CHAT_HANDLE" \
  golang:1.23 go run ./cmd/client
