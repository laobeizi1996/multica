DROP TRIGGER IF EXISTS trg_workspace_seed_defaults ON workspace;
DROP FUNCTION IF EXISTS seed_workspace_defaults();

DROP TABLE IF EXISTS issue_to_project;
DROP TABLE IF EXISTS project_to_label;
DROP TABLE IF EXISTS project_label;
DROP TABLE IF EXISTS project;

DROP TABLE IF EXISTS knowledge_capture_run;
DROP TABLE IF EXISTS workspace_knowledge_repo;

ALTER TABLE issue DROP CONSTRAINT IF EXISTS issue_id_workspace_unique;
