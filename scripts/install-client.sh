#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DEFAULT_INSTALL_DIR=${INSTALL_DIR:-/usr/local/bin}
CONFIG_DIR=${XDG_CONFIG_HOME:-"$HOME/.config"}/teamchat

if [ ! -x "$ROOT_DIR/dist/termichat" ]; then
  sh "$ROOT_DIR/scripts/build-client-linux.sh"
fi

INSTALL_DIR=$DEFAULT_INSTALL_DIR
USE_SUDO=""
if [ ! -w "$INSTALL_DIR" ]; then
  if command -v sudo >/dev/null 2>&1; then
    USE_SUDO="sudo"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

mkdir -p "$CONFIG_DIR"
if [ "$INSTALL_DIR" = "$HOME/.local/bin" ]; then
  mkdir -p "$INSTALL_DIR"
else
  $USE_SUDO mkdir -p "$INSTALL_DIR"
fi
mkdir -p "$CONFIG_DIR"
$USE_SUDO rm -f "$INSTALL_DIR/teamchat"
$USE_SUDO cp "$ROOT_DIR/dist/termichat" "$INSTALL_DIR/termichat"
$USE_SUDO chmod +x "$INSTALL_DIR/termichat"
cat > "$CONFIG_DIR/client.env" <<'EOF'
CHAT_SERVER_URL=http://termichat.zeraynce.com
CHAT_WORKSPACE=acme
CHAT_WORKSPACE_CODE=acme123
CHAT_DEFAULT_CHANNEL=lobby
EOF

printf 'installed termichat to %s/termichat\n' "$INSTALL_DIR"
printf 'saved config to %s/client.env\n' "$CONFIG_DIR"
printf 'run with: termichat\n'
