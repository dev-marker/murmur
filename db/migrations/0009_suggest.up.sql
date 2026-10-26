-- Who to follow starts from the most followed accounts. Without this index
-- every ask reads all of users.
create index users_followers_count_idx on users (followers_count desc);
