-- name: ListProjectsByWorkspace :many
SELECT * FROM project
WHERE workspace_id = $1
ORDER BY created_at ASC;

-- name: GetProjectInWorkspace :one
SELECT * FROM project
WHERE id = $1 AND workspace_id = $2;

-- name: CreateProject :one
INSERT INTO project (
    workspace_id,
    parent_id,
    name,
    slug,
    description,
    kind,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: UpdateProject :one
UPDATE project SET
    parent_id = sqlc.narg('parent_id'),
    name = COALESCE(sqlc.narg('name'), name),
    slug = COALESCE(sqlc.narg('slug'), slug),
    description = COALESCE(sqlc.narg('description'), description),
    kind = COALESCE(sqlc.narg('kind'), kind),
    status = COALESCE(sqlc.narg('status'), status),
    updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: DeleteProject :exec
DELETE FROM project
WHERE id = $1 AND workspace_id = $2;

-- name: GetDefaultProjectForWorkspace :one
SELECT * FROM project
WHERE workspace_id = $1 AND kind = 'general'
ORDER BY created_at ASC
LIMIT 1;

-- name: ListProjectLabelsByWorkspace :many
SELECT * FROM project_label
WHERE workspace_id = $1
ORDER BY name ASC;

-- name: GetProjectLabelInWorkspace :one
SELECT * FROM project_label
WHERE id = $1 AND workspace_id = $2;

-- name: CreateProjectLabel :one
INSERT INTO project_label (
    workspace_id,
    name,
    color
) VALUES (
    $1, $2, $3
)
RETURNING *;

-- name: UpdateProjectLabel :one
UPDATE project_label SET
    name = COALESCE(sqlc.narg('name'), name),
    color = COALESCE(sqlc.narg('color'), color),
    updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: DeleteProjectLabel :exec
DELETE FROM project_label
WHERE id = $1 AND workspace_id = $2;

-- name: ListProjectLabelsForProject :many
SELECT pl.*
FROM project_label pl
JOIN project_to_label ptl ON ptl.label_id = pl.id
WHERE ptl.project_id = $1
ORDER BY pl.name ASC;

-- name: DeleteProjectLabelsForProject :exec
DELETE FROM project_to_label
WHERE project_id = $1;

-- name: AddProjectLabelToProject :exec
INSERT INTO project_to_label (
    workspace_id,
    project_id,
    label_id
) VALUES (
    $1, $2, $3
)
ON CONFLICT (project_id, label_id) DO NOTHING;

-- name: ListIssueProjectsWithPrimary :many
SELECT p.*, itp.is_primary
FROM project p
JOIN issue_to_project itp ON itp.project_id = p.id
WHERE itp.issue_id = $1
ORDER BY itp.is_primary DESC, p.created_at ASC;

-- name: DeleteIssueProjects :exec
DELETE FROM issue_to_project
WHERE issue_id = $1;

-- name: AddIssueProject :exec
INSERT INTO issue_to_project (
    issue_id,
    project_id,
    workspace_id,
    is_primary
) VALUES (
    $1, $2, $3, $4
)
ON CONFLICT (issue_id, project_id)
DO UPDATE SET is_primary = EXCLUDED.is_primary;
