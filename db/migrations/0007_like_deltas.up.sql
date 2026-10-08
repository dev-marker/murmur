-- Changes to a like count that nobody has added up yet.
create table like_deltas (
    id      bigint generated always as identity primary key,
    post_id bigint   not null,
    delta   smallint not null
);
