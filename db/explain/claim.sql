-- What one claim costs: make explain Q=claim
explain (analyze, costs off, timing off)
select id
from jobs
where status = 'waiting' and run_at <= now()
order by run_at, id
limit 1
for update skip locked;

-- The same claim without today's index. The index is dropped inside a transaction
-- that is rolled back, so it never really goes away. The table is locked meanwhile.
begin;
drop index jobs_run_at_id_idx;
explain (analyze, costs off, timing off)
select id
from jobs
where status = 'waiting' and run_at <= now()
order by run_at, id
limit 1
for update skip locked;
rollback;
