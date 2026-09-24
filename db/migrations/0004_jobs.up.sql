create table jobs (
    id         bigint generated always as identity primary key,
    kind       text        not null,
    payload    jsonb       not null,
    status     text        not null default 'waiting' check (status in ('waiting', 'running')),
    created_at timestamptz not null default now()
);
