create table if not exists subscription_quotas (
    email varchar(255) primary key,
    max_subscriptions integer not null,
    used_slots integer not null default 0,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists quota_reservations (
    id bigserial primary key,
    saga_id uuid not null,
    subscription_id bigint not null unique,
    email varchar(255) not null references subscription_quotas(email),
    status varchar(32) not null,
    rejection_reason text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (saga_id)
);

create index if not exists idx_quota_reservations_email_status
    on quota_reservations (email, status);
