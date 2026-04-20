#!/usr/bin/env sh
set -eu

INSTALL_DIR=${INSTALL_DIR:-"$HOME/.local/bin"}
mkdir -p "$INSTALL_DIR"
cp "$(dirname "$0")/teamchat" "$INSTALL_DIR/teamchat"
chmod +x "$INSTALL_DIR/teamchat"
printf 'installed to %s/teamchat\n' "$INSTALL_DIR"
printf 'start with:\n'
printf '  CHAT_SERVER_URL=http://termichat.zeraynce.com:8080 CHAT_WORKSPACE=acme CHAT_WORKSPACE_CODE=acme123 teamchat\n'
printf 'add %s to PATH if needed\n' "$INSTALL_DIR"
