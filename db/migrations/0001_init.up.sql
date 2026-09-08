create table users (
    id         bigint generated always as identity primary key,
    handle     text        not null unique,
    token_hash bytea       not null unique,
    created_at timestamptz not null default now()
);

create table posts (
    id         bigint generated always as identity primary key,
    user_id    bigint      not null references users (id),
    body       text        not null check (char_length(body) between 1 and 280),
    created_at timestamptz not null default now()
);

create index posts_user_id_id_idx on posts (user_id, id desc);
