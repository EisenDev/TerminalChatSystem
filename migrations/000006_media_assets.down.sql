drop table if exists media_assets;

alter table messages drop constraint if exists messages_message_type_check;

alter table messages
    add constraint messages_message_type_check
    check (message_type in ('chat', 'system', 'emote'));
