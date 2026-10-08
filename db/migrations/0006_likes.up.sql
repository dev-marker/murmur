-- A like is a fact: who liked which post.
create table likes (
    post_id    bigint      not null references posts (id),
    user_id    bigint      not null references users (id),
    created_at timestamptz not null default now(),
    primary key (post_id, user_id)
);

alter table posts add column likes_count integer not null default 0;
