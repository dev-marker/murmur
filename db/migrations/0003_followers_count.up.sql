alter table users add column followers_count integer not null default 0;

-- Users who already have followers get their count from the follows table.
update users u
set followers_count = c.n
from (select followee_id, count(*) as n from follows group by followee_id) c
where u.id = c.followee_id;
