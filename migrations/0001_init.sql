create extension if not exists pgcrypto;

create table if not exists checklists (
    id uuid primary key default gen_random_uuid(),
    status text not null check (status in ('running', 'succeeded', 'failed')),
    task_artifact_key text not null,
    context_artifact_key text not null,
    error_message text null,
    created_at timestamptz not null default now(),
    completed_at timestamptz null
);

create table if not exists questions (
    checklist_id uuid not null references checklists(id) on delete restrict,
    id text not null,
    ordinal integer not null check (ordinal > 0),
    rationale text not null,
    question text not null,
    primary key (checklist_id, id),
    unique (checklist_id, ordinal)
);

create table if not exists weights (
    checklist_id uuid not null,
    question_id text not null,
    rationale text not null,
    weight integer not null check (weight >= 0 and weight <= 4),
    primary key (checklist_id, question_id),
    foreign key (checklist_id, question_id) references questions(checklist_id, id) on delete restrict
);

create table if not exists evaluations (
    id uuid primary key default gen_random_uuid(),
    checklist_id uuid not null references checklists(id) on delete restrict,
    status text not null check (status in ('running', 'succeeded', 'failed')),
    answer_artifact_key text not null,
    satisfied_points integer null check (satisfied_points is null or satisfied_points >= 0),
    total_possible_points integer null check (total_possible_points is null or total_possible_points > 0),
    checklist_pass_rate double precision null check (checklist_pass_rate is null or (checklist_pass_rate >= 0 and checklist_pass_rate <= 1)),
    error_message text null,
    created_at timestamptz not null default now(),
    completed_at timestamptz null,
    unique (id, checklist_id)
);

create table if not exists judgments (
    evaluation_id uuid not null,
    checklist_id uuid not null,
    question_id text not null,
    evidence text not null check (length(btrim(evidence)) > 0),
    answer text not null check (answer in ('yes', 'no')),
    primary key (evaluation_id, question_id),
    foreign key (evaluation_id, checklist_id) references evaluations(id, checklist_id) on delete restrict,
    foreign key (checklist_id, question_id) references questions(checklist_id, id) on delete restrict
);

create or replace function prevent_terminal_checklist_update()
returns trigger
language plpgsql
as $$
begin
    if old.status in ('succeeded', 'failed') then
        raise exception 'terminal checklists are immutable';
    end if;
    return new;
end;
$$;

drop trigger if exists checklists_terminal_immutable on checklists;
create trigger checklists_terminal_immutable
before update on checklists
for each row execute function prevent_terminal_checklist_update();

create or replace function prevent_terminal_evaluation_update()
returns trigger
language plpgsql
as $$
begin
    if old.status in ('succeeded', 'failed') then
        raise exception 'terminal evaluations are immutable';
    end if;
    return new;
end;
$$;

drop trigger if exists evaluations_terminal_immutable on evaluations;
create trigger evaluations_terminal_immutable
before update on evaluations
for each row execute function prevent_terminal_evaluation_update();

