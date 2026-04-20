#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
INSTALL_DIR=${INSTALL_DIR:-"$HOME/.local/bin"}

if [ ! -x "$ROOT_DIR/dist/teamchat" ]; then
  sh "$ROOT_DIR/scripts/build-client-linux.sh"
fi

mkdir -p "$INSTALL_DIR"
cp "$ROOT_DIR/dist/teamchat" "$INSTALL_DIR/teamchat"
chmod +x "$INSTALL_DIR/teamchat"

printf 'installed teamchat to %s/teamchat\n' "$INSTALL_DIR"
printf 'run with: CHAT_SERVER_URL=http://SERVER:18080 CHAT_WORKSPACE=acme teamchat\n'
