create table if not exists tracked_repositories (
    id bigserial primary key,
    full_name varchar(512) not null unique,
    last_seen_tag varchar(255),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);
