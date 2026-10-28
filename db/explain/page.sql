-- The posts of one page of a feed, planned the way our servers plan it: make plan Q=page U=42
set plan_cache_mode = force_generic_plan;
prepare page(bigint[], bigint) as
select p.id, u.handle, p.body, p.created_at, p.likes_count,
	exists (select 1 from likes where post_id = p.id and user_id = $2)
from posts p
join users u on u.id = p.user_id
where p.id = any($1)
order by p.id desc;
-- the ids of the page: the newest 21 posts of the people the reader follows
select array_agg(id) as ids from (
    select p.id from follows f
    cross join lateral (select id from posts where user_id = f.followee_id order by id desc limit 21) p
    where f.follower_id = :user order by p.id desc limit 21) page \gset
explain (analyze, timing off) execute page(:'ids', :user);
