-- Who to follow, planned the way our servers plan it: make explain Q=suggest U=42
set plan_cache_mode = force_generic_plan;
prepare suggest(bigint) as
select handle, followers_count
from (select id, handle, followers_count from users
    order by followers_count desc limit 50) top
where id <> $1
  and not exists (
    select 1 from follows where follower_id = $1 and followee_id = top.id)
order by followers_count desc
limit 3;
explain (analyze, costs off, timing off) execute suggest(:user);
