#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

: "${CHAT_DATABASE_URL:=postgres://teamchat:teamchat@localhost:15432/teamchat?sslmode=disable}"
: "${CHAT_REDIS_ADDR:=localhost:6379}"
: "${CHAT_HTTP_ADDR:=:18080}"

docker rm -f teamchat-server >/dev/null 2>&1 || true

exec docker run --rm --name teamchat-server --network host \
  -v "$ROOT_DIR:/app" -w /app \
  -e "CHAT_DATABASE_URL=$CHAT_DATABASE_URL" \
  -e "CHAT_REDIS_ADDR=$CHAT_REDIS_ADDR" \
  -e "CHAT_HTTP_ADDR=$CHAT_HTTP_ADDR" \
  golang:1.23 go run ./cmd/server
