create table if not exists mail_outbox (
    id bigserial primary key,
    recipient_email varchar(255) not null,
    subject varchar(255) not null,
    html_body text not null,
    text_body text not null,
    attempts integer not null default 0,
    processing_started_at timestamptz,
    sent_at timestamptz,
    last_error text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists idx_mail_outbox_pending
    on mail_outbox (sent_at, processing_started_at, id);
