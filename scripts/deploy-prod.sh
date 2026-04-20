#!/usr/bin/env sh
set -eu

APP_DIR=${APP_DIR:-/opt/teamchat}
REPO_URL=${REPO_URL:-https://github.com/EisenDev/TerminalChatSystem.git}
BRANCH=${BRANCH:-main}

sudo mkdir -p "$(dirname "$APP_DIR")"

if [ ! -d "$APP_DIR/.git" ]; then
  sudo rm -rf "$APP_DIR"
  git clone --branch "$BRANCH" "$REPO_URL" "$APP_DIR"
else
  cd "$APP_DIR"
  git fetch origin "$BRANCH"
  git checkout "$BRANCH"
  git reset --hard "origin/$BRANCH"
fi

cd "$APP_DIR"

if [ -n "${GHCR_USERNAME:-}" ] && [ -n "${GHCR_TOKEN:-}" ]; then
  printf '%s' "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USERNAME" --password-stdin
fi

docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d postgres redis

until docker compose -f docker-compose.prod.yml exec -T postgres pg_isready -U teamchat -d teamchat >/dev/null 2>&1; do
  sleep 2
done

docker compose -f docker-compose.prod.yml exec -T postgres psql -U teamchat -d teamchat < migrations/000001_init.up.sql
docker compose -f docker-compose.prod.yml exec -T postgres psql -U teamchat -d teamchat < migrations/000002_emotes.up.sql
docker compose -f docker-compose.prod.yml exec -T postgres psql -U teamchat -d teamchat < scripts/seed.sql

docker compose -f docker-compose.prod.yml up -d app
