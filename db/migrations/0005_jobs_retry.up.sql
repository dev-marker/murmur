alter table jobs
    add column attempts   integer     not null default 0,
    add column run_at     timestamptz not null default now(),
    add column claimed_at timestamptz,
    add column last_error text;

alter table jobs
    drop constraint jobs_status_check,
    add constraint jobs_status_check check (status in ('waiting', 'running', 'dead'));

-- A job that is running right now was claimed now, as far as we know.
update jobs set claimed_at = now() where status = 'running';

create index jobs_run_at_id_idx on jobs (run_at, id) where status = 'waiting';
