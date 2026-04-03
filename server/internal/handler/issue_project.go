package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type IssueProjectResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	ParentID    *string `json:"parent_id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description string  `json:"description"`
	Kind        string  `json:"kind"`
	Status      string  `json:"status"`
}

func issueProjectRowToResponse(row db.ListIssueProjectsWithPrimaryRow) IssueProjectResponse {
	return IssueProjectResponse{
		ID:          uuidToString(row.ID),
		WorkspaceID: uuidToString(row.WorkspaceID),
		ParentID:    uuidToPtr(row.ParentID),
		Name:        row.Name,
		Slug:        row.Slug,
		Description: row.Description,
		Kind:        row.Kind,
		Status:      row.Status,
	}
}

func applyIssueProjects(resp *IssueResponse, rows []db.ListIssueProjectsWithPrimaryRow) {
	projects := make([]IssueProjectResponse, 0, len(rows))
	var primary *string
	for _, row := range rows {
		projects = append(projects, issueProjectRowToResponse(row))
		if row.IsPrimary {
			id := uuidToString(row.ID)
			primary = &id
		}
	}
	resp.Projects = projects
	resp.PrimaryProjectID = primary
}

func (h *Handler) attachIssueProjects(ctx context.Context, resp *IssueResponse, issueID pgtype.UUID) {
	rows, err := h.Queries.ListIssueProjectsWithPrimary(ctx, issueID)
	if err != nil {
		resp.Projects = []IssueProjectResponse{}
		resp.PrimaryProjectID = nil
		return
	}
	applyIssueProjects(resp, rows)
}

func normalizeProjectIDList(projectIDs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(projectIDs))
	for _, id := range projectIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func (h *Handler) setIssueProjects(ctx context.Context, q *db.Queries, issue db.Issue, projectIDs []string, primaryProjectID *string) ([]db.ListIssueProjectsWithPrimaryRow, error) {
	normalized := normalizeProjectIDList(projectIDs)
	if len(normalized) == 0 {
		defaultProject, err := q.GetDefaultProjectForWorkspace(ctx, issue.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("default project not found")
		}
		normalized = []string{uuidToString(defaultProject.ID)}
	}

	validated := make([]pgtype.UUID, 0, len(normalized))
	selected := map[string]bool{}
	for _, projectID := range normalized {
		project, err := q.GetProjectInWorkspace(ctx, db.GetProjectInWorkspaceParams{
			ID:          parseUUID(projectID),
			WorkspaceID: issue.WorkspaceID,
		})
		if err != nil {
			return nil, fmt.Errorf("project %s does not belong to the workspace", projectID)
		}
		id := uuidToString(project.ID)
		selected[id] = true
		validated = append(validated, project.ID)
	}

	primaryID := ""
	if primaryProjectID != nil && strings.TrimSpace(*primaryProjectID) != "" {
		primaryID = strings.TrimSpace(*primaryProjectID)
		if !selected[primaryID] {
			return nil, fmt.Errorf("primary_project_id must be included in project_ids")
		}
	} else {
		primaryID = uuidToString(validated[0])
	}

	if err := q.DeleteIssueProjects(ctx, issue.ID); err != nil {
		return nil, err
	}

	for _, projectID := range validated {
		pid := uuidToString(projectID)
		if err := q.AddIssueProject(ctx, db.AddIssueProjectParams{
			IssueID:     issue.ID,
			ProjectID:   projectID,
			WorkspaceID: issue.WorkspaceID,
			IsPrimary:   pid == primaryID,
		}); err != nil {
			return nil, err
		}
	}

	return q.ListIssueProjectsWithPrimary(ctx, issue.ID)
}
