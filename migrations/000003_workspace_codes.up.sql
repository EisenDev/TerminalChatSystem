alter table workspaces add column if not exists join_code text;
alter table workspaces add column if not exists owner_handle text;

update workspaces
set join_code = coalesce(join_code, 'acme123'),
    owner_handle = coalesce(owner_handle, 'system')
where join_code is null or owner_handle is null;

alter table workspaces alter column join_code set not null;
alter table workspaces alter column owner_handle set not null;
