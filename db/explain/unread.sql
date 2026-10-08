-- The unread count, for one user: make explain Q=unread U=1
-- First the way the API asks: it stops counting at a hundred.
explain (analyze, costs off, timing off)
select count(*) from (
    select 1
    from notifications
    where user_id = :user and id > (select seen_notification from users where id = :user)
    limit 100
) unseen;

-- And then without the limit: every unread notification is counted.
explain (analyze, costs off, timing off)
select count(*)
from notifications
where user_id = :user and id > (select seen_notification from users where id = :user);
