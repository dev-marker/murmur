-- The pull, for one reader: make explain Q=pull U=363
explain (analyze, costs off, timing off)
select p.id
from follows f
join users u on u.id = f.followee_id and u.followers_count >= 1000
cross join lateral (
    select id
    from posts
    where user_id = f.followee_id and id < 9223372036854775807
    order by id desc
    limit 21
) p
where f.follower_id = :user
order by p.id desc
limit 21;
