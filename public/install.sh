#!/usr/bin/env sh
set -eu

BASE_URL=${TEAMCHAT_BASE_URL:-http://termichat.zeraynce.com}
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

ARCHIVE="$TMP_DIR/teamchat-client-linux-amd64.tar.gz"

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$BASE_URL/downloads/teamchat-client-linux-amd64.tar.gz" -o "$ARCHIVE"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$ARCHIVE" "$BASE_URL/downloads/teamchat-client-linux-amd64.tar.gz"
else
  echo "curl or wget is required"
  exit 1
fi

tar -xzf "$ARCHIVE" -C "$TMP_DIR"
sh "$TMP_DIR/install.sh"

echo
echo "Run:"
echo "  termichat"
