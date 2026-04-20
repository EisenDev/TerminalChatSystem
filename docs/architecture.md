# Architecture

## Folder Structure

```text
/cmd/client                Bubble Tea terminal app entrypoint
/cmd/server                HTTP/WebSocket chat server entrypoint
/internal/client/ui        Bubble Tea model, rendering, keyboard flow
/internal/client/ws        Client WebSocket connection manager and reconnect loop
/internal/client/state     Client-side app state
/internal/client/call      Voice/call UI scaffolding
/internal/server/httpapi   HTTP server and health endpoint
/internal/server/ws        Gorilla WebSocket transport
/internal/server/chat      Chat hub and session orchestration
/internal/server/presence  Presence tracking with Redis-ready publishing
/internal/server/store     PostgreSQL store
/internal/server/auth      Handle validation
/internal/server/call      Voice/call signaling scaffold
/internal/shared/protocol  Shared WebSocket event protocol
/internal/shared/models    Shared domain models
/internal/shared/config    Env-based configuration
/internal/shared/logging   Structured logging
/migrations                SQL schema migrations
/scripts                   Bootstrap and seed helpers
/docs                      Architecture and future notes
```

## Runtime Design

- The server exposes `/healthz` and `/ws`.
- Each WebSocket connection owns one chat session.
- The chat hub serializes session registration, disconnects, and inbound events through channels to avoid race-heavy shared-state code.
- PostgreSQL remains the source of truth for users, workspaces, channels, membership, and messages.
- Presence is kept in memory for the single-node MVP and optionally mirrored into Redis keys/pubsub for future multi-node fanout.
- The Bubble Tea client keeps a small local state model and talks JSON envelopes over WebSocket.

## Tradeoffs

- Public channels are fully functional in the MVP. Direct messages are scaffolded at the command and schema level but not fully materialized into direct-channel lifecycle flows yet.
- Redis is not required for single-node local development. The package is integrated now so horizontal-scale hooks exist without forcing the MVP to depend on pubsub correctness.
- `pgx` is used directly to keep the codebase lean. If query volume or SQL surface area grows, swapping to `sqlc` is straightforward because the storage API is already isolated.
