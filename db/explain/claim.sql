-- What one claim costs: make explain Q=claim
explain (analyze, costs off, timing off)
select id
from jobs
where status = 'waiting'
order by id
limit 1
for update skip locked;
