$ErrorActionPreference = "Stop"

if (-not $env:CHAT_DATABASE_URL) {
  $env:CHAT_DATABASE_URL = "postgres://teamchat:teamchat@localhost:5432/teamchat?sslmode=disable"
}

if (-not (Get-Command migrate -ErrorAction SilentlyContinue)) {
  Write-Error "migrate CLI not found"
}

if (-not (Get-Command psql -ErrorAction SilentlyContinue)) {
  Write-Error "psql not found"
}

migrate -path migrations -database $env:CHAT_DATABASE_URL up
psql $env:CHAT_DATABASE_URL -f scripts/seed.sql
Write-Host "bootstrap complete"
