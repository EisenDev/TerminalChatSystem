#!/usr/bin/env sh
set -eu

BASE_URL=${TEAMCHAT_BASE_URL:-http://termichat.zeraynce.com}
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

TARGET_BIN=${TARGET_BIN:-$(command -v termichat 2>/dev/null || true)}
if [ -z "$TARGET_BIN" ]; then
  if [ -x "$HOME/.local/bin/termichat" ]; then
    TARGET_BIN="$HOME/.local/bin/termichat"
  else
    echo "termichat is not installed yet. Use /install.sh first."
    exit 1
  fi
fi

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

if [ ! -w "$(dirname "$TARGET_BIN")" ]; then
  if command -v sudo >/dev/null 2>&1; then
    sudo cp "$TMP_DIR/termichat" "$TARGET_BIN"
    sudo chmod +x "$TARGET_BIN"
  else
    echo "Need write access to $(dirname "$TARGET_BIN") to update termichat."
    exit 1
  fi
else
  cp "$TMP_DIR/termichat" "$TARGET_BIN"
  chmod +x "$TARGET_BIN"
fi

echo "Updated termichat at $TARGET_BIN"
echo "Run:"
echo "  rehash"
echo "  termichat"
