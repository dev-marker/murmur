-- Pick the oldest job that is waiting: make claim
select id, kind
from jobs
where status = 'waiting'
order by id
limit 1
for update skip locked;
