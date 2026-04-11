create table if not exists subscriptions (
    id bigserial primary key,
    user_id bigint not null references users(id),
    tracked_repository_id bigint not null references tracked_repositories(id) on delete cascade,
    confirmed boolean not null default false,
    confirmation_token uuid not null default gen_random_uuid(),
    cancellation_token uuid not null default gen_random_uuid(),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (user_id, tracked_repository_id)
)