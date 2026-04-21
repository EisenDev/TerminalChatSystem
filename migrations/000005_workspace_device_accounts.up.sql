create table if not exists workspace_device_accounts (
    id uuid primary key default gen_random_uuid(),
    workspace_id uuid not null references workspaces(id) on delete cascade,
    device_fingerprint text not null,
    user_id uuid not null references users(id) on delete cascade,
    created_at timestamptz not null default now(),
    last_seen_at timestamptz not null default now(),
    unique (workspace_id, device_fingerprint),
    unique (workspace_id, user_id)
);

create index if not exists idx_workspace_device_accounts_last_seen
    on workspace_device_accounts (workspace_id, last_seen_at desc);
