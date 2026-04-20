drop index if exists idx_device_accounts_last_seen;
drop table if exists device_accounts;
alter table workspaces drop column if exists owner_user_id;
