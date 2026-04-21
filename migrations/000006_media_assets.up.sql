alter table messages drop constraint if exists messages_message_type_check;

alter table messages
    add constraint messages_message_type_check
    check (message_type in ('chat', 'system', 'emote', 'media'));

create table if not exists media_assets (
    id uuid primary key default gen_random_uuid(),
    workspace_id uuid not null references workspaces(id) on delete cascade,
    channel_id uuid not null references channels(id) on delete cascade,
    message_id uuid not null unique references messages(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    kind text not null check (kind in ('image', 'video', 'file')),
    object_key text not null unique,
    file_name text not null,
    content_type text not null,
    byte_size bigint not null,
    created_at timestamptz not null default now()
);

create index if not exists idx_media_assets_channel_created_at on media_assets (channel_id, created_at desc);
