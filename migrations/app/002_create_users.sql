create table if not exists users (
    id bigserial primary key,
    email varchar(255) not null unique,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);