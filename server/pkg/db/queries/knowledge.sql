-- name: GetWorkspaceKnowledgeRepo :one
SELECT * FROM workspace_knowledge_repo
WHERE workspace_id = $1;

-- name: UpsertWorkspaceKnowledgeRepo :one
INSERT INTO workspace_knowledge_repo (
    workspace_id,
    repo_url,
    default_branch,
    curator_agent_id,
    template_version,
    mode,
    enabled,
    last_bootstrapped_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (workspace_id) DO UPDATE SET
    repo_url = EXCLUDED.repo_url,
    default_branch = EXCLUDED.default_branch,
    curator_agent_id = EXCLUDED.curator_agent_id,
    template_version = EXCLUDED.template_version,
    mode = EXCLUDED.mode,
    enabled = EXCLUDED.enabled,
    last_bootstrapped_at = EXCLUDED.last_bootstrapped_at,
    updated_at = now()
RETURNING *;

-- name: MarkKnowledgeRepoBootstrapped :one
UPDATE workspace_knowledge_repo
SET template_version = $2,
    last_bootstrapped_at = now(),
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: CreateKnowledgeCaptureRun :one
INSERT INTO knowledge_capture_run (
    workspace_id,
    issue_id,
    trigger_source,
    status,
    dedupe_status,
    merged_into_run_id,
    task_id,
    pr_url,
    skip_reason,
    error,
    started_at,
    finished_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: GetActiveKnowledgeCaptureRunByIssue :one
SELECT * FROM knowledge_capture_run
WHERE workspace_id = $1
  AND issue_id = $2
  AND status IN ('pending', 'running')
ORDER BY created_at DESC
LIMIT 1;

-- name: GetKnowledgeCaptureRunByTaskID :one
SELECT * FROM knowledge_capture_run
WHERE task_id = $1;

-- name: MarkKnowledgeCaptureRunRunning :one
UPDATE knowledge_capture_run
SET status = 'running',
    started_at = now()
WHERE task_id = $1
RETURNING *;

-- name: MarkKnowledgeCaptureRunCompleted :one
UPDATE knowledge_capture_run
SET status = 'completed',
    pr_url = sqlc.narg('pr_url'),
    skip_reason = NULL,
    error = NULL,
    finished_at = now()
WHERE task_id = $1
RETURNING *;

-- name: MarkKnowledgeCaptureRunSkipped :one
UPDATE knowledge_capture_run
SET status = 'skipped',
    skip_reason = $2,
    error = NULL,
    finished_at = now()
WHERE task_id = $1
RETURNING *;

-- name: MarkKnowledgeCaptureRunFailed :one
UPDATE knowledge_capture_run
SET status = 'failed',
    error = $2,
    finished_at = now()
WHERE task_id = $1
RETURNING *;
