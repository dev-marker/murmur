-- News for a user: somebody did something to one of their posts.
create table notifications (
    id         bigint generated always as identity primary key,
    user_id    bigint      not null references users (id),
    kind       text        not null,
    actor_id   bigint      not null references users (id),
    post_id    bigint      not null references posts (id),
    created_at timestamptz not null default now(),
    constraint notifications_once unique (post_id, actor_id, kind)
);

create index notifications_user_id_id_idx on notifications (user_id, id desc);

-- Every notification up to this id has been seen by its user.
alter table users add column seen_notification bigint not null default 0;
