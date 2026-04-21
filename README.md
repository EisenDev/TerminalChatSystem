# Terminal Team Chat

Cross-platform terminal chat for Windows and Linux, built in Go with Bubble Tea, Gorilla WebSocket, PostgreSQL, and Redis-ready presence hooks. The MVP is text-chat first and leaves clean signaling scaffolding for future voice calling with Pion WebRTC.

## Final Folder Structure

```text
teamchat/
  cmd/
    client/
    server/
  internal/
    client/
      call/
      state/
      ui/
      ws/
    server/
      auth/
      call/
      chat/
      httpapi/
      presence/
      store/
      ws/
    shared/
      config/
      logging/
      models/
      protocol/
  migrations/
  scripts/
  docs/
  .env.example
  docker-compose.yml
  Makefile
  go.mod
```

## Architecture Brief

- `cmd/server` starts the HTTP API, WebSocket endpoint, chat hub, PostgreSQL store, and presence manager.
- `internal/server/chat` is the core orchestration layer. It validates session flow, joins workspaces/channels, persists messages, and broadcasts updates.
- `internal/shared/protocol` defines the typed JSON event envelopes used by both client and server.
- `cmd/client` launches a Bubble Tea UI with a setup flow, message pane, channels pane, coworker pane, footer notifications, and reconnecting WebSocket manager.
- Voice is scaffolded behind dedicated `call` packages so Pion signaling/media can be added without rewiring chat transport.

## Features In This MVP

- Terminal-first chat UX
- Device-bound workspace join flow
- Public channels
- Keyboard channel switching from the sidebar
- Real-time messaging
- Message history loading
- Presence updates
- Live handle changing
- User-to-user ping notifications
- Terminal emotes with lightweight animation
- Distinct deterministic user colors
- Right-aligned self messages
- Reconnect loop on the client
- Structured logging
- PostgreSQL persistence
- Redis-ready presence cache and pubsub hooks
- 7-day history visibility window for new joins
- Future voice/call protocol and UI scaffolding

## Local Run

### Prerequisites

- Go 1.23+
- PostgreSQL 16+
- Redis 7+
- `migrate` CLI
- `psql`

### Linux and Windows setup

1. Copy `.env.example` into your shell environment.
2. Start PostgreSQL and Redis.
3. Run migrations.
4. Seed the default workspace/channel.
5. Start the server.
6. Start one or more clients.

### Fast path with Docker

```bash
docker compose up -d
```

### Export environment

Linux/macOS:

```bash
export CHAT_DATABASE_URL='postgres://teamchat:teamchat@localhost:5432/teamchat?sslmode=disable'
export CHAT_REDIS_ADDR='localhost:6379'
export CHAT_SERVER_URL='http://localhost:8080'
export CHAT_WORKSPACE='acme'
export CHAT_WORKSPACE_CODE='acme123'
```

Windows PowerShell:

```powershell
$env:CHAT_DATABASE_URL = "postgres://teamchat:teamchat@localhost:5432/teamchat?sslmode=disable"
$env:CHAT_REDIS_ADDR = "localhost:6379"
$env:CHAT_SERVER_URL = "http://localhost:8080"
$env:CHAT_WORKSPACE = "acme"
$env:CHAT_WORKSPACE_CODE = "acme123"
```

### Run migrations and seed

```bash
make bootstrap
```

Windows PowerShell:

```powershell
./scripts/bootstrap.ps1
```

Equivalent manual commands:

```bash
migrate -path migrations -database "$CHAT_DATABASE_URL" up
psql "$CHAT_DATABASE_URL" -f scripts/seed.sql
```

### Start the server

```bash
make server
```

Docker helper:

```bash
make run-server-docker
```

### Optional media uploads with Cloudflare R2

Set these on the server when you want `/image`, `/video`, and `/file` uploads to work:

```bash
export CHAT_PUBLIC_BASE_URL='https://termichat.zeraynce.com'
export CHAT_R2_ENDPOINT='https://<accountid>.r2.cloudflarestorage.com'
export CHAT_R2_ACCESS_KEY='<r2-access-key-id>'
export CHAT_R2_SECRET_KEY='<r2-secret-access-key>'
export CHAT_R2_BUCKET='termichat'
export CHAT_R2_PUBLIC_BASE='https://pub.termichat.zeraynce.com'
export CHAT_MEDIA_MAX_BYTES='26214400'
```

The server uploads media into R2, stores a media record in PostgreSQL, and serves a public viewer page at `/pub/<media-id>`. The client commands are:

- `/image /path/to/file.png`
- `/video /path/to/file.mp4`
- `/file /path/to/file.zip`

### Start the client

```bash
make client
```

Docker helper:

```bash
make run-client-docker
```

Enter:

- server URL, such as `http://localhost:8080`
- workspace, such as `acme`
- workspace code, such as `acme123`
- handle, such as `alice`, only the first time that device joins

The first user to join a brand-new workspace name creates it, sets its code, and becomes the workspace owner. Later users must use the same workspace name plus the correct code. The client stores a local device token under `~/.config/teamchat/profile.json`; the server combines that token with the connection IP into a hashed device fingerprint, so the raw IP is not exposed through the protocol or normal session logs. Once a device has joined successfully, the next join to that workspace can reuse the remembered handle automatically. The client joins the default `lobby` channel and starts receiving real-time updates.

New joins only receive the latest 7 days of channel history.

### Local multi-client testing

Run the server once, then open multiple terminal windows and start one client per window.

Client 1:

```bash
CHAT_HANDLE=alice CHAT_SERVER_URL=http://localhost:8080 CHAT_WORKSPACE=acme CHAT_WORKSPACE_CODE=acme123 make client
```

Client 2:

```bash
CHAT_HANDLE=bob CHAT_SERVER_URL=http://localhost:8080 CHAT_WORKSPACE=acme CHAT_WORKSPACE_CODE=acme123 make client
```

For LAN testing, replace `localhost` with the server machine IP, such as `http://192.168.1.25:8080`.

### Installable client binary

Build a Linux client binary:

```bash
make build-client-linux
```

Install it locally into your normal command path:

```bash
make install-client
```

Create a friend-share package:

```bash
make package-client-linux
```

This creates:

```text
dist/termichat-linux-amd64.tar.gz
```

Your friend can unpack it and run:

```bash
sh install.sh
termichat
```

Or install directly from the hosted script:

```bash
curl -fsSL http://termichat.zeraynce.com/install.sh | sh
termichat
```

## Keyboard / Commands

- `Enter`: send message
- `Tab` / left / right: switch active pane
- `Up` / `Down` in the channels pane: move channel selection
- `Enter` in the channels pane: switch channel
- `Ctrl+R`: reconnect
- `Ctrl+C`: quit
- `?`: toggle help

Slash commands:

- `/join <channel>`
- `/dm <user>`
- `/users`
- `/channels`
- `/ping <user> [--flash|--fku]`
- `/ping all [--flash|--fku]`
- `/effects <on|off>`
- `/muteeffects <handle>`
- `/chandle <new_handle>`
- `/emote`
- `/me <action>`
- `/clear`
- `/quit`
- `/call <user>` scaffold only
- `/mute` scaffold only
- `/hangup` scaffold only

## UI Notes

- The header uses a pixel-style ASCII banner and collapses on smaller terminals.
- Your own messages render on the right side of the message pane.
- The coworker pane shows online status and the last known active channel from presence updates.
- Pinged users are temporarily highlighted in the coworker list.
- Emotes are sent as special chat messages and animated lightly in the viewport.
- Ping effects render as temporary overlays on top of the UI.
- `/effects off` disables local ping overlays.
- `/muteeffects <handle>` toggles whether one sender can trigger overlays on your client.

## WebSocket Protocol

Client to server:

- `identify`
- `join_workspace`
- `join_channel`
- `send_message`
- `send_emote`
- `request_history`
- `ping`
- `ping_user`
- `ping_all`
- `typing_start`
- `typing_stop`
- `request_users`
- `request_channels`
- `change_handle`
- `call_invite`
- `call_accept`
- `call_reject`
- `call_hangup`
- `mute_state_changed`

Server to client:

- `identified`
- `workspace_joined`
- `channel_joined`
- `message_new`
- `emote_new`
- `history_batch`
- `presence_update`
- `users_list`
- `channels_list`
- `user_joined`
- `user_left`
- `typing_update`
- `ping_received`
- `ping_effect`
- `handle_changed`
- `system_notice`
- `error`
- `pong`
- `reconnect_required`
- `call_invitation`
- `call_state_update`

## Voice Scaffolds

These pieces are intentionally placeholders for later Pion WebRTC work:

- [internal/server/call/manager.go](/home/eisen/teamchat/internal/server/call/manager.go)
- [internal/client/call/state.go](/home/eisen/teamchat/internal/client/call/state.go)
- [docs/voice-future.md](/home/eisen/teamchat/docs/voice-future.md)
- `future_calls` migration table
- call-related protocol events

## Tests

Included tests cover:

- protocol envelope round-trip
- handle validation
- basic chat hub message flow

Run:

```bash
make test
```

## Notes

- The app is intentionally trust-based right now: there is no real authentication or transport encryption yet.
- Run migrations again after pulling this phase because message types now include `emote`.
- I validated this repo with `go test ./...`, `go build ./cmd/server`, and `go build ./cmd/client` in a temporary Go 1.23 container.
- Ping effects are server-routed, client-rendered, and server rate-limited to reduce spam.
