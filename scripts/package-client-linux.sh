#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PACKAGE_DIR="$ROOT_DIR/dist/package"

sh "$ROOT_DIR/scripts/build-client-linux.sh"

rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR"

cp "$ROOT_DIR/dist/teamchat" "$PACKAGE_DIR/teamchat"

cat > "$PACKAGE_DIR/install.sh" <<'EOF'
#!/usr/bin/env sh
set -eu

INSTALL_DIR=${INSTALL_DIR:-"$HOME/.local/bin"}
mkdir -p "$INSTALL_DIR"
cp "$(dirname "$0")/teamchat" "$INSTALL_DIR/teamchat"
chmod +x "$INSTALL_DIR/teamchat"
printf 'installed to %s/teamchat\n' "$INSTALL_DIR"
printf 'add %s to PATH if needed\n' "$INSTALL_DIR"
EOF

chmod +x "$PACKAGE_DIR/teamchat" "$PACKAGE_DIR/install.sh"

cat > "$PACKAGE_DIR/README.txt" <<'EOF'
TEAMCHAT CLIENT PACKAGE

1. Run:
   sh install.sh

2. Start the client:
   CHAT_SERVER_URL=http://YOUR_SERVER_IP:18080 CHAT_WORKSPACE=acme teamchat

3. If teamchat is not found, add ~/.local/bin to PATH:
   export PATH="$HOME/.local/bin:$PATH"
EOF

cd "$ROOT_DIR/dist"
tar -czf teamchat-client-linux-amd64.tar.gz -C "$PACKAGE_DIR" .
printf 'created %s/dist/teamchat-client-linux-amd64.tar.gz\n' "$ROOT_DIR"
