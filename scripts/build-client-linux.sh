#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

mkdir -p "$ROOT_DIR/dist"

exec docker run --rm \
  -v "$ROOT_DIR:/app" -w /app \
  -e GOOS=linux \
  -e GOARCH=amd64 \
  -e CGO_ENABLED=0 \
  golang:1.23 \
  go build -buildvcs=false -o dist/teamchat ./cmd/client
