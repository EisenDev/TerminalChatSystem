#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
CONFIG_DIR=${XDG_CONFIG_HOME:-"$HOME/.config"}/teamchat
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

if [ ! -x "$ROOT_DIR/dist/termichat" ]; then
  sh "$ROOT_DIR/scripts/build-client-linux.sh"
fi

mkdir -p "$CONFIG_DIR"
mkdir -p "$INSTALL_DIR"
mkdir -p "$CONFIG_DIR"
rm -f "$INSTALL_DIR/teamchat"
cp "$ROOT_DIR/dist/termichat" "$INSTALL_DIR/termichat"
chmod +x "$INSTALL_DIR/termichat"
cat > "$CONFIG_DIR/client.env" <<'EOF'
CHAT_SERVER_URL=http://termichat.zeraynce.com
CHAT_WORKSPACE=acme
CHAT_WORKSPACE_CODE=acme123
CHAT_DEFAULT_CHANNEL=lobby
EOF

printf 'installed termichat to %s/termichat\n' "$INSTALL_DIR"
printf 'saved config to %s/client.env\n' "$CONFIG_DIR"
printf 'run with: termichat\n'
