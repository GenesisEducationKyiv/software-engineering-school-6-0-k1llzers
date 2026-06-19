create table if not exists message_inbox (
    id bigserial primary key,
    message_id uuid not null unique,
    message_type varchar(255) not null,
    payload_json jsonb not null,
    processed_at timestamptz,
    processing_started_at timestamptz,
    created_at timestamptz not null default now()
);
