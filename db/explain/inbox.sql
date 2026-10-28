-- One page of notifications, planned the way our servers plan it: make plan Q=inbox U=1
set plan_cache_mode = force_generic_plan;
prepare inbox(bigint, bigint, int) as
select n.id, n.kind, a.handle, n.post_id, n.created_at, n.id > u.seen_notification,
    p.body
from notifications n
join users a on a.id = n.actor_id
join users u on u.id = n.user_id
join posts p on p.id = n.post_id
where n.user_id = $1 and n.id < $2
order by n.id desc
limit $3;
explain (analyze, timing off) execute inbox(:user, 9223372036854775807, 21);
