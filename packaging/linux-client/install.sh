#!/usr/bin/env sh
set -eu

DEFAULT_INSTALL_DIR=${INSTALL_DIR:-/usr/local/bin}
CONFIG_DIR=${XDG_CONFIG_HOME:-"$HOME/.config"}/teamchat
SHELL_NAME=$(basename "${SHELL:-sh}")
INSTALL_DIR=$DEFAULT_INSTALL_DIR
USE_SUDO=""
if [ ! -w "$INSTALL_DIR" ]; then
  if command -v sudo >/dev/null 2>&1; then
    USE_SUDO="sudo"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

if [ "$INSTALL_DIR" = "$HOME/.local/bin" ]; then
  mkdir -p "$INSTALL_DIR"
else
  $USE_SUDO mkdir -p "$INSTALL_DIR"
fi
cp "$(dirname "$0")/termichat" /tmp/termichat-install-bin
$USE_SUDO rm -f "$INSTALL_DIR/teamchat"
$USE_SUDO cp /tmp/termichat-install-bin "$INSTALL_DIR/termichat"
$USE_SUDO chmod +x "$INSTALL_DIR/termichat"
rm -f /tmp/termichat-install-bin
mkdir -p "$CONFIG_DIR"
cat > "$CONFIG_DIR/client.env" <<'EOF'
CHAT_SERVER_URL=http://termichat.zeraynce.com
CHAT_WORKSPACE=acme
CHAT_WORKSPACE_CODE=acme123
CHAT_DEFAULT_CHANNEL=lobby
EOF
ensure_path_line() {
  target_file=$1
  if [ -f "$target_file" ] && grep -Fq 'export PATH="$HOME/.local/bin:$PATH"' "$target_file"; then
    return
  fi
  mkdir -p "$(dirname "$target_file")"
  printf '\nexport PATH="$HOME/.local/bin:$PATH"\n' >> "$target_file"
}

case "$SHELL_NAME" in
  zsh)
    ensure_path_line "$HOME/.zshrc"
    ;;
  bash)
    ensure_path_line "$HOME/.bashrc"
    ;;
  *)
    ensure_path_line "$HOME/.profile"
    ;;
esac
printf 'installed to %s/termichat\n' "$INSTALL_DIR"
printf 'saved config to %s/client.env\n' "$CONFIG_DIR"
printf 'start with:\n'
printf '  termichat\n'
if [ "$INSTALL_DIR" = "$HOME/.local/bin" ]; then
  printf 'if this terminal does not find termichat yet, run:\n'
  printf '  export PATH="$HOME/.local/bin:$PATH"\n'
fi
