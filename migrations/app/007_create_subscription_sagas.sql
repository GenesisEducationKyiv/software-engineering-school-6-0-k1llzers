create table if not exists subscription_sagas (
    id uuid primary key,
    subscription_id bigint not null,
    operation varchar(32) not null,
    status varchar(32) not null,
    attempts integer not null default 0,
    last_error text,
    processing_started_at timestamptz,
    next_retry_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create unique index if not exists idx_subscription_sagas_active_operation
    on subscription_sagas (subscription_id, operation)
    where status not in ('completed', 'compensated', 'dead', 'rejected');

create index if not exists idx_subscription_sagas_retry
    on subscription_sagas (status, next_retry_at, processing_started_at, created_at);

insert into subscription_sagas (id, subscription_id, operation, status)
select gen_random_uuid(), s.id, 'subscribe', 'completed'
from subscriptions s
where not exists (
    select 1
    from subscription_sagas ss
    where ss.subscription_id = s.id
      and ss.operation = 'subscribe'
);
