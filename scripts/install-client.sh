#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
INSTALL_DIR=${INSTALL_DIR:-"$HOME/.local/bin"}
CONFIG_DIR=${XDG_CONFIG_HOME:-"$HOME/.config"}/teamchat

if [ ! -x "$ROOT_DIR/dist/teamchat" ]; then
  sh "$ROOT_DIR/scripts/build-client-linux.sh"
fi

mkdir -p "$INSTALL_DIR"
mkdir -p "$CONFIG_DIR"
cp "$ROOT_DIR/dist/teamchat" "$INSTALL_DIR/teamchat"
chmod +x "$INSTALL_DIR/teamchat"
cat > "$CONFIG_DIR/client.env" <<'EOF'
CHAT_SERVER_URL=http://termichat.zeraynce.com
CHAT_WORKSPACE=acme
CHAT_WORKSPACE_CODE=acme123
CHAT_DEFAULT_CHANNEL=lobby
EOF

printf 'installed teamchat to %s/teamchat\n' "$INSTALL_DIR"
printf 'saved config to %s/client.env\n' "$CONFIG_DIR"
printf 'run with: termichat\n'
