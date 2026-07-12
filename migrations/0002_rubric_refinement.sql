drop table if exists judgments cascade;
drop table if exists evaluations cascade;
drop table if exists weights cascade;
drop table if exists question_weights cascade;
drop table if exists questions cascade;
drop table if exists candidate_questions cascade;
drop table if exists checklist_dimensions cascade;
drop table if exists checklists cascade;

create extension if not exists pgcrypto;

create table checklists (
    id uuid primary key default gen_random_uuid(),
    status text not null check (status in ('running', 'succeeded', 'failed')),
    task_artifact_key text not null,
    context_artifact_key text not null,
    error_message text null,
    created_at timestamptz not null default now(),
    completed_at timestamptz null
);

create table checklist_dimensions (
    checklist_id uuid not null references checklists(id) on delete restrict,
    id text not null,
    ordinal integer not null check (ordinal > 0),
    name text not null check (length(btrim(name)) > 0),
    rubric text not null check (length(btrim(rubric)) > 0),
    rationale text not null check (length(btrim(rationale)) > 0),
    primary key (checklist_id, id),
    unique (checklist_id, ordinal)
);

create table candidate_questions (
    checklist_id uuid not null,
    id text not null,
    dimension_id text not null,
    ordinal integer not null check (ordinal > 0),
    rationale text not null check (length(btrim(rationale)) > 0),
    question text not null check (length(btrim(question)) > 0),
    primary key (checklist_id, id),
    unique (checklist_id, ordinal),
    foreign key (checklist_id, dimension_id) references checklist_dimensions(checklist_id, id) on delete restrict
);

create table question_weights (
    checklist_id uuid not null,
    candidate_question_id text not null,
    rationale text not null check (length(btrim(rationale)) > 0),
    weight integer not null check (weight >= 0 and weight <= 4),
    primary key (checklist_id, candidate_question_id),
    foreign key (checklist_id, candidate_question_id) references candidate_questions(checklist_id, id) on delete restrict
);

create table questions (
    checklist_id uuid not null,
    id text not null,
    ordinal integer not null check (ordinal > 0),
    dimension_id text not null,
    source_candidate_id text not null,
    rationale text not null check (length(btrim(rationale)) > 0),
    question text not null check (length(btrim(question)) > 0),
    primary key (checklist_id, id),
    unique (checklist_id, ordinal),
    foreign key (checklist_id, dimension_id) references checklist_dimensions(checklist_id, id) on delete restrict,
    foreign key (checklist_id, source_candidate_id) references candidate_questions(checklist_id, id) on delete restrict
);

create table evaluations (
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

create table judgments (
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

create trigger evaluations_terminal_immutable
before update on evaluations
for each row execute function prevent_terminal_evaluation_update();
