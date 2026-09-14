-- The feed query, for one user: make explain U=42
explain (analyze, costs off, timing off)
select p.id, u.handle, p.body, p.created_at
from follows f
cross join lateral (
    select id, user_id, body, created_at
    from posts
    where user_id = f.followee_id and id < 9223372036854775807
    order by id desc
    limit 21
) p
join users u on u.id = p.user_id
where f.follower_id = :user
order by p.id desc
limit 21;
