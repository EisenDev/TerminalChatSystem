#!/usr/bin/env sh
set -eu

BASE_URL=${TEAMCHAT_BASE_URL:-http://termichat.zeraynce.com}
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

TARGET_BIN=${TARGET_BIN:-"$HOME/.local/bin/termichat"}
TARGET_DIR=$(dirname "$TARGET_BIN")
mkdir -p "$TARGET_DIR"

if [ ! -x "$TARGET_BIN" ] && [ -n "$(command -v termichat 2>/dev/null || true)" ] && [ "$(command -v termichat)" != "$TARGET_BIN" ]; then
  echo "termichat is installed outside the user-local path."
  echo "This updater only manages $HOME/.local/bin/termichat."
  echo "Reinstall with /install.sh to move it to the user-local path."
  exit 1
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
install -m 0755 "$TMP_DIR/termichat" "$TARGET_BIN.new"
mv -f "$TARGET_BIN.new" "$TARGET_BIN"

echo "Updated termichat at $TARGET_BIN"
echo "Run:"
echo "  rehash"
echo "  termichat"
