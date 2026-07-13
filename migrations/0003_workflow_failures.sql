alter table checklists drop column error_message;
alter table evaluations drop column error_message;

create table workflow_failures (
    id uuid primary key default gen_random_uuid(),
    checklist_id uuid null references checklists(id) on delete restrict,
    evaluation_id uuid null references evaluations(id) on delete restrict,
    workflow_id text not null check (length(btrim(workflow_id)) > 0),
    stage text not null check (length(btrim(stage)) > 0),
    error_class text not null check (error_class in ('model_output_invalid', 'infra_retryable', 'infra_non_retryable')),
    error_code text not null check (length(btrim(error_code)) > 0),
    message text not null check (length(btrim(message)) > 0),
    retryable boolean not null,
    attempt_count integer not null check (attempt_count > 0),
    diagnostics jsonb not null default '[]'::jsonb check (jsonb_typeof(diagnostics) = 'array'),
    artifact_references jsonb not null default '[]'::jsonb check (jsonb_typeof(artifact_references) = 'array'),
    created_at timestamptz not null default now(),
    constraint workflow_failures_exactly_one_entity check ((checklist_id is null) <> (evaluation_id is null))
);

create unique index workflow_failures_checklist_unique
    on workflow_failures (checklist_id) where checklist_id is not null;

create unique index workflow_failures_evaluation_unique
    on workflow_failures (evaluation_id) where evaluation_id is not null;

create or replace function prevent_workflow_failure_update()
returns trigger
language plpgsql
as $$
begin
    raise exception 'workflow failures are immutable';
end;
$$;

create trigger workflow_failures_immutable
before update or delete on workflow_failures
for each row execute function prevent_workflow_failure_update();
