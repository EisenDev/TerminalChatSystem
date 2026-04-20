create extension if not exists pgcrypto;

create table if not exists users (
    id uuid primary key default gen_random_uuid(),
    handle text not null unique,
    display_name text not null,
    created_at timestamptz not null default now()
);

create table if not exists workspaces (
    id uuid primary key default gen_random_uuid(),
    name text not null unique,
    created_at timestamptz not null default now()
);

create table if not exists workspace_members (
    id uuid primary key default gen_random_uuid(),
    workspace_id uuid not null references workspaces(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    joined_at timestamptz not null default now(),
    unique (workspace_id, user_id)
);

create table if not exists channels (
    id uuid primary key default gen_random_uuid(),
    workspace_id uuid not null references workspaces(id) on delete cascade,
    name text not null,
    kind text not null check (kind in ('public', 'direct', 'system')),
    created_at timestamptz not null default now(),
    unique (workspace_id, name)
);

create table if not exists channel_members (
    id uuid primary key default gen_random_uuid(),
    channel_id uuid not null references channels(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    joined_at timestamptz not null default now(),
    unique (channel_id, user_id)
);

create table if not exists messages (
    id uuid primary key default gen_random_uuid(),
    workspace_id uuid not null references workspaces(id) on delete cascade,
    channel_id uuid not null references channels(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    body text not null,
    message_type text not null check (message_type in ('chat', 'system')),
    created_at timestamptz not null default now()
);

create index if not exists idx_messages_channel_created_at on messages (channel_id, created_at desc);

create table if not exists sessions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    workspace_id uuid references workspaces(id) on delete cascade,
    connected_at timestamptz not null default now(),
    last_seen_at timestamptz not null default now(),
    connection_meta jsonb not null default '{}'::jsonb
);

create table if not exists future_calls (
    id uuid primary key default gen_random_uuid(),
    workspace_id uuid not null references workspaces(id) on delete cascade,
    channel_id uuid references channels(id) on delete set null,
    initiator_id uuid not null references users(id) on delete cascade,
    status text not null default 'idle',
    metadata jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now()
);
