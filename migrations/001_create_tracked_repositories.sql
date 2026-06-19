create table if not exists tracked_repositories (
    id bigserial primary key,
    owner varchar(40) not null,
    name varchar(100) not null,
    last_seen_tag varchar(255),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (owner, name)
);
