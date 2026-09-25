drop index jobs_run_at_id_idx;

-- Two statuses again, so a dead job has to go.
delete from jobs where status = 'dead';

alter table jobs
    drop constraint jobs_status_check,
    add constraint jobs_status_check check (status in ('waiting', 'running')),
    drop column attempts,
    drop column run_at,
    drop column claimed_at,
    drop column last_error;
