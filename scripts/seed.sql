insert into workspaces (name, join_code, owner_handle)
values ('acme', 'acme123', 'system')
on conflict (name) do nothing;

insert into channels (workspace_id, name, kind)
select w.id, 'lobby', 'public'
from workspaces w
where w.name = 'acme'
on conflict (workspace_id, name) do nothing;
