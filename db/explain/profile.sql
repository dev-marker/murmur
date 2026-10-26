-- One profile, planned the way our servers plan it: make explain Q=profile U=1
set plan_cache_mode = force_generic_plan;
prepare profile(text, bigint) as
select u.handle, u.created_at, u.followers_count,
    (select count(*) from follows where follower_id = u.id),
    (select count(*) from posts where user_id = u.id),
    exists (select 1 from follows where follower_id = $2 and followee_id = u.id)
from users u
where u.handle = $1;
explain (analyze, costs off, timing off) execute profile('user' || :user, 42);
