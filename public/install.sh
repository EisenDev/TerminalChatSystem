#!/usr/bin/env sh
set -eu

BASE_URL=${TEAMCHAT_BASE_URL:-https://termichat.zeraynce.com}
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

ARCHIVE="$TMP_DIR/termichat-linux-amd64.tar.gz"
ARCHIVE_URL="$BASE_URL/downloads/termichat-linux-amd64.tar.gz?ts=$(date +%s)"

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$ARCHIVE_URL" -o "$ARCHIVE"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$ARCHIVE" "$ARCHIVE_URL"
else
  echo "curl or wget is required"
  exit 1
fi

tar -xzf "$ARCHIVE" -C "$TMP_DIR"
sh "$TMP_DIR/install.sh"

echo
echo "Run:"
echo "  termichat"
