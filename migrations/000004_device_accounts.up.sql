alter table workspaces add column if not exists owner_user_id uuid references users(id) on delete set null;

update workspaces w
set owner_user_id = u.id
from users u
where w.owner_user_id is null
  and lower(u.handle) = lower(w.owner_handle);

create table if not exists device_accounts (
    id uuid primary key default gen_random_uuid(),
    device_fingerprint text not null unique,
    user_id uuid not null unique references users(id) on delete cascade,
    created_at timestamptz not null default now(),
    last_seen_at timestamptz not null default now()
);

create index if not exists idx_device_accounts_last_seen on device_accounts (last_seen_at desc);
