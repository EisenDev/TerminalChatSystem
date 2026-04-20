#!/usr/bin/env sh
set -eu

: "${CHAT_DATABASE_URL:=postgres://teamchat:teamchat@localhost:5432/teamchat?sslmode=disable}"

if ! command -v migrate >/dev/null 2>&1; then
  echo "migrate CLI not found"
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found"
  exit 1
fi

migrate -path migrations -database "$CHAT_DATABASE_URL" up
psql "$CHAT_DATABASE_URL" -f scripts/seed.sql
echo "bootstrap complete"
