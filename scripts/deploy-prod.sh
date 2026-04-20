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
docker compose -f docker-compose.prod.yml up -d postgres redis

until docker compose -f docker-compose.prod.yml exec -T postgres pg_isready -U teamchat -d teamchat >/dev/null 2>&1; do
  sleep 2
done

docker compose -f docker-compose.prod.yml exec -T postgres psql -U teamchat -d teamchat < migrations/000001_init.up.sql
docker compose -f docker-compose.prod.yml exec -T postgres psql -U teamchat -d teamchat < migrations/000002_emotes.up.sql
docker compose -f docker-compose.prod.yml exec -T postgres psql -U teamchat -d teamchat < scripts/seed.sql

docker compose -f docker-compose.prod.yml up -d app
