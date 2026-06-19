create table if not exists integration_outbox (
    id bigserial primary key,
    message_id uuid not null unique,
    message_type varchar(255) not null,
    payload_json jsonb not null,
    processing_started_at timestamptz,
    published_at timestamptz,
    created_at timestamptz not null default now()
);

create index if not exists idx_integration_outbox_pending
    on integration_outbox (published_at, processing_started_at, id);
