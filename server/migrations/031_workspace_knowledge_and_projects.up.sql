CREATE TABLE workspace_knowledge_repo (
    workspace_id UUID PRIMARY KEY REFERENCES workspace(id) ON DELETE CASCADE,
    repo_url TEXT NOT NULL DEFAULT '',
    default_branch TEXT NOT NULL DEFAULT 'main',
    curator_agent_id UUID REFERENCES agent(id) ON DELETE SET NULL,
    template_version TEXT NOT NULL DEFAULT 'openai-harness-v1',
    mode TEXT NOT NULL DEFAULT 'pr' CHECK (mode IN ('pr')),
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_bootstrapped_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO workspace_knowledge_repo (
    workspace_id,
    repo_url,
    default_branch,
    curator_agent_id,
    template_version,
    mode,
    enabled
)
SELECT
    w.id,
    '',
    'main',
    NULL,
    'openai-harness-v1',
    'pr',
    true
FROM workspace w
ON CONFLICT (workspace_id) DO NOTHING;

CREATE TABLE knowledge_capture_run (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    trigger_source TEXT NOT NULL CHECK (trigger_source IN ('task_completed', 'issue_done')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'skipped', 'failed', 'deduplicated')),
    dedupe_status TEXT NOT NULL DEFAULT 'leader' CHECK (dedupe_status IN ('leader', 'merged')),
    merged_into_run_id UUID REFERENCES knowledge_capture_run(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    pr_url TEXT,
    skip_reason TEXT,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_knowledge_capture_workspace_issue_created
    ON knowledge_capture_run (workspace_id, issue_id, created_at DESC);
CREATE UNIQUE INDEX idx_knowledge_capture_active_issue
    ON knowledge_capture_run (workspace_id, issue_id)
    WHERE status IN ('pending', 'running');
CREATE UNIQUE INDEX idx_knowledge_capture_task
    ON knowledge_capture_run (task_id)
    WHERE task_id IS NOT NULL;

CREATE TABLE project (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    parent_id UUID,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL CHECK (kind IN ('portfolio', 'epic', 'theme', 'general')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, slug),
    UNIQUE (id, workspace_id),
    CONSTRAINT project_parent_fk
        FOREIGN KEY (parent_id, workspace_id)
        REFERENCES project(id, workspace_id)
        ON DELETE SET NULL
);

CREATE INDEX idx_project_workspace_parent ON project (workspace_id, parent_id);

CREATE TABLE project_label (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    color TEXT NOT NULL DEFAULT 'blue',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name),
    UNIQUE (id, workspace_id)
);

CREATE TABLE project_to_label (
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    project_id UUID NOT NULL,
    label_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, label_id),
    CONSTRAINT project_to_label_project_fk
        FOREIGN KEY (project_id, workspace_id)
        REFERENCES project(id, workspace_id)
        ON DELETE CASCADE,
    CONSTRAINT project_to_label_label_fk
        FOREIGN KEY (label_id, workspace_id)
        REFERENCES project_label(id, workspace_id)
        ON DELETE CASCADE
);

ALTER TABLE issue
    ADD CONSTRAINT issue_id_workspace_unique UNIQUE (id, workspace_id);

CREATE TABLE issue_to_project (
    issue_id UUID NOT NULL,
    project_id UUID NOT NULL,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, project_id),
    CONSTRAINT issue_to_project_issue_fk
        FOREIGN KEY (issue_id, workspace_id)
        REFERENCES issue(id, workspace_id)
        ON DELETE CASCADE,
    CONSTRAINT issue_to_project_project_fk
        FOREIGN KEY (project_id, workspace_id)
        REFERENCES project(id, workspace_id)
        ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_issue_to_project_primary
    ON issue_to_project (issue_id)
    WHERE is_primary = true;
CREATE INDEX idx_issue_to_project_project ON issue_to_project (project_id);
CREATE INDEX idx_issue_to_project_workspace ON issue_to_project (workspace_id, issue_id);

INSERT INTO project (workspace_id, name, slug, description, kind, status)
SELECT
    w.id,
    'General',
    'general',
    'General workspace project',
    'general',
    'active'
FROM workspace w
ON CONFLICT (workspace_id, slug) DO NOTHING;

INSERT INTO issue_to_project (issue_id, project_id, workspace_id, is_primary)
SELECT
    i.id,
    p.id,
    i.workspace_id,
    true
FROM issue i
JOIN project p
    ON p.workspace_id = i.workspace_id
    AND p.slug = 'general'
    AND p.kind = 'general'
LEFT JOIN issue_to_project itp
    ON itp.issue_id = i.id
WHERE itp.issue_id IS NULL;

CREATE OR REPLACE FUNCTION seed_workspace_defaults()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO workspace_knowledge_repo (
        workspace_id,
        repo_url,
        default_branch,
        template_version,
        mode,
        enabled
    ) VALUES (
        NEW.id,
        '',
        'main',
        'openai-harness-v1',
        'pr',
        true
    )
    ON CONFLICT (workspace_id) DO NOTHING;

    INSERT INTO project (workspace_id, name, slug, description, kind, status)
    VALUES (NEW.id, 'General', 'general', 'General workspace project', 'general', 'active')
    ON CONFLICT (workspace_id, slug) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_workspace_seed_defaults
AFTER INSERT ON workspace
FOR EACH ROW
EXECUTE FUNCTION seed_workspace_defaults();
