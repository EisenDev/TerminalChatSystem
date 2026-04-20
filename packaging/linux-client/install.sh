#!/usr/bin/env sh
set -eu

INSTALL_DIR=${INSTALL_DIR:-"$HOME/.local/bin"}
CONFIG_DIR=${XDG_CONFIG_HOME:-"$HOME/.config"}/teamchat
mkdir -p "$INSTALL_DIR"
cp "$(dirname "$0")/teamchat" "$INSTALL_DIR/teamchat"
chmod +x "$INSTALL_DIR/teamchat"
mkdir -p "$CONFIG_DIR"
cat > "$CONFIG_DIR/client.env" <<'EOF'
CHAT_SERVER_URL=http://termichat.zeraynce.com
CHAT_WORKSPACE=acme
CHAT_WORKSPACE_CODE=acme123
CHAT_DEFAULT_CHANNEL=lobby
EOF
printf 'installed to %s/teamchat\n' "$INSTALL_DIR"
printf 'saved config to %s/client.env\n' "$CONFIG_DIR"
printf 'start with:\n'
printf '  termichat\n'
printf 'add %s to PATH if needed\n' "$INSTALL_DIR"
