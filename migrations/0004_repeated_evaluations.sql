alter table checklists
    add column evaluation_runs integer not null default 3
    check (evaluation_runs > 0 and evaluation_runs % 2 = 1);

alter table checklists disable trigger checklists_terminal_immutable;

update checklists
set evaluation_runs = 1
where exists (
    select 1 from evaluations where evaluations.checklist_id = checklists.id
);

alter table checklists enable trigger checklists_terminal_immutable;

alter table judgments
    add column run_index integer not null default 1 check (run_index > 0);

alter table judgments drop constraint judgments_pkey;
alter table judgments add primary key (evaluation_id, run_index, question_id);
alter table judgments alter column run_index drop default;
