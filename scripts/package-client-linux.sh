#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PACKAGE_DIR="$ROOT_DIR/dist/package"

sh "$ROOT_DIR/scripts/build-client-linux.sh"

rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR"

cp "$ROOT_DIR/dist/teamchat" "$PACKAGE_DIR/teamchat"
cp "$ROOT_DIR/packaging/linux-client/install.sh" "$PACKAGE_DIR/install.sh"
cp "$ROOT_DIR/packaging/linux-client/README.txt" "$PACKAGE_DIR/README.txt"

chmod +x "$PACKAGE_DIR/teamchat" "$PACKAGE_DIR/install.sh"

cd "$ROOT_DIR/dist"
tar -czf teamchat-client-linux-amd64.tar.gz -C "$PACKAGE_DIR" .
printf 'created %s/dist/teamchat-client-linux-amd64.tar.gz\n' "$ROOT_DIR"
