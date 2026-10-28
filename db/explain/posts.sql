-- A user's posts, planned the way our servers plan it: make plan Q=posts U=1
set plan_cache_mode = force_generic_plan;
prepare posts(text, bigint, int, bigint) as
select p.id, p.user_id, p.body, p.created_at, u.handle, p.likes_count,
    exists (select 1 from likes where post_id = p.id and user_id = $4)
from posts p
join users u on u.id = p.user_id
where p.user_id = (select id from users where handle = $1) and p.id < $2
order by p.id desc
limit $3;
explain (analyze, timing off) execute posts('user' || :user, 9223372036854775807, 21, 42);
