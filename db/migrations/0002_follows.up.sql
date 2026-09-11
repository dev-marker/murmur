create table follows (
    follower_id bigint      not null references users (id),
    followee_id bigint      not null references users (id),
    created_at  timestamptz not null default now(),
    primary key (follower_id, followee_id),
    constraint no_self_follow check (follower_id <> followee_id)
);

create index follows_followee_id_follower_id_idx on follows (followee_id, follower_id);
