#!/usr/bin/env sh
set -eu

APP_DIR=${APP_DIR:-/opt/teamchat}

mkdir -p "$APP_DIR"
cd "$APP_DIR"

if [ ! -f docker-compose.prod.yml ]; then
  echo "docker-compose.prod.yml not found in $APP_DIR"
  exit 1
fi

docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
