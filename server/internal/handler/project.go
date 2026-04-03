package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var projectSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

type ProjectLabelResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ProjectResponse struct {
	ID          string                 `json:"id"`
	WorkspaceID string                 `json:"workspace_id"`
	ParentID    *string                `json:"parent_id"`
	Name        string                 `json:"name"`
	Slug        string                 `json:"slug"`
	Description string                 `json:"description"`
	Kind        string                 `json:"kind"`
	Status      string                 `json:"status"`
	Labels      []ProjectLabelResponse `json:"labels"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

type ProjectTreeNode struct {
	Project  ProjectResponse    `json:"project"`
	Children []*ProjectTreeNode `json:"children"`
}

func projectLabelToResponse(label db.ProjectLabel) ProjectLabelResponse {
	return ProjectLabelResponse{
		ID:          uuidToString(label.ID),
		WorkspaceID: uuidToString(label.WorkspaceID),
		Name:        label.Name,
		Color:       label.Color,
		CreatedAt:   timestampToString(label.CreatedAt),
		UpdatedAt:   timestampToString(label.UpdatedAt),
	}
}

func projectToResponse(project db.Project) ProjectResponse {
	return ProjectResponse{
		ID:          uuidToString(project.ID),
		WorkspaceID: uuidToString(project.WorkspaceID),
		ParentID:    uuidToPtr(project.ParentID),
		Name:        project.Name,
		Slug:        project.Slug,
		Description: project.Description,
		Kind:        project.Kind,
		Status:      project.Status,
		Labels:      []ProjectLabelResponse{},
		CreatedAt:   timestampToString(project.CreatedAt),
		UpdatedAt:   timestampToString(project.UpdatedAt),
	}
}

func normalizeProjectSlug(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = projectSlugPattern.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	if v == "" {
		return "project"
	}
	return v
}

func normalizeProjectKind(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "theme", true
	}
	switch v {
	case "portfolio", "epic", "theme", "general":
		return v, true
	default:
		return "", false
	}
}

func normalizeProjectStatus(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "active", true
	}
	switch v {
	case "active", "archived":
		return v, true
	default:
		return "", false
	}
}

func (h *Handler) loadProjectLabelsForProject(r *http.Request, projectID pgtype.UUID) ([]ProjectLabelResponse, error) {
	labels, err := h.Queries.ListProjectLabelsForProject(r.Context(), projectID)
	if err != nil {
		return nil, err
	}
	resp := make([]ProjectLabelResponse, 0, len(labels))
	for _, label := range labels {
		resp = append(resp, projectLabelToResponse(label))
	}
	return resp, nil
}

func (h *Handler) setProjectLabels(r *http.Request, workspaceID string, projectID pgtype.UUID, labelIDs []string) error {
	if err := h.Queries.DeleteProjectLabelsForProject(r.Context(), projectID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, labelID := range labelIDs {
		id := strings.TrimSpace(labelID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := h.Queries.GetProjectLabelInWorkspace(r.Context(), db.GetProjectLabelInWorkspaceParams{
			ID:          parseUUID(id),
			WorkspaceID: parseUUID(workspaceID),
		}); err != nil {
			return err
		}
		if err := h.Queries.AddProjectLabelToProject(r.Context(), db.AddProjectLabelToProjectParams{
			WorkspaceID: parseUUID(workspaceID),
			ProjectID:   projectID,
			LabelID:     parseUUID(id),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	projects, err := h.Queries.ListProjectsByWorkspace(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	resp := make([]ProjectResponse, 0, len(projects))
	for _, project := range projects {
		item := projectToResponse(project)
		labels, err := h.loadProjectLabelsForProject(r, project.ID)
		if err == nil {
			item.Labels = labels
		}
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, resp)
}

type CreateProjectRequest struct {
	ParentID    *string  `json:"parent_id"`
	Name        string   `json:"name"`
	Slug        *string  `json:"slug"`
	Description *string  `json:"description"`
	Kind        *string  `json:"kind"`
	Status      *string  `json:"status"`
	LabelIDs    []string `json:"label_ids"`
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	slug := normalizeProjectSlug(name)
	if req.Slug != nil && strings.TrimSpace(*req.Slug) != "" {
		slug = normalizeProjectSlug(*req.Slug)
	}

	kindInput := ""
	if req.Kind != nil {
		kindInput = *req.Kind
	}
	kind, ok := normalizeProjectKind(kindInput)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid kind")
		return
	}

	statusInput := ""
	if req.Status != nil {
		statusInput = *req.Status
	}
	status, ok := normalizeProjectStatus(statusInput)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	var parentID pgtype.UUID
	if req.ParentID != nil && strings.TrimSpace(*req.ParentID) != "" {
		parentID = parseUUID(strings.TrimSpace(*req.ParentID))
		if _, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{
			ID:          parentID,
			WorkspaceID: parseUUID(workspaceID),
		}); err != nil {
			writeError(w, http.StatusBadRequest, "parent_id must belong to the workspace")
			return
		}
	}

	project, err := h.Queries.CreateProject(r.Context(), db.CreateProjectParams{
		WorkspaceID: parseUUID(workspaceID),
		ParentID:    parentID,
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(valueOrDefault(req.Description, "")),
		Kind:        kind,
		Status:      status,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "project slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	if err := h.setProjectLabels(r, workspaceID, project.ID, req.LabelIDs); err != nil {
		_ = h.Queries.DeleteProject(r.Context(), db.DeleteProjectParams{
			ID:          project.ID,
			WorkspaceID: parseUUID(workspaceID),
		})
		writeError(w, http.StatusBadRequest, "invalid label_ids")
		return
	}

	resp := projectToResponse(project)
	if labels, err := h.loadProjectLabelsForProject(r, project.ID); err == nil {
		resp.Labels = labels
	}
	writeJSON(w, http.StatusCreated, resp)
}

type UpdateProjectRequest struct {
	ParentID    *string  `json:"parent_id"`
	Name        *string  `json:"name"`
	Slug        *string  `json:"slug"`
	Description *string  `json:"description"`
	Kind        *string  `json:"kind"`
	Status      *string  `json:"status"`
	LabelIDs    []string `json:"label_ids"`
}

func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	projectID := chi.URLParam(r, "projectId")

	current, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{
		ID:          parseUUID(projectID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var req UpdateProjectRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(bodyBytes, &raw)

	params := db.UpdateProjectParams{
		ID:          current.ID,
		WorkspaceID: current.WorkspaceID,
		ParentID:    current.ParentID,
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		params.Name = pgtype.Text{String: name, Valid: true}
	}
	if req.Slug != nil {
		params.Slug = pgtype.Text{String: normalizeProjectSlug(*req.Slug), Valid: true}
	}
	if req.Description != nil {
		params.Description = pgtype.Text{String: strings.TrimSpace(*req.Description), Valid: true}
	}
	if req.Kind != nil {
		kind, ok := normalizeProjectKind(*req.Kind)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid kind")
			return
		}
		params.Kind = pgtype.Text{String: kind, Valid: true}
	}
	if req.Status != nil {
		status, ok := normalizeProjectStatus(*req.Status)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		params.Status = pgtype.Text{String: status, Valid: true}
	}
	if _, exists := raw["parent_id"]; exists {
		if req.ParentID == nil || strings.TrimSpace(*req.ParentID) == "" {
			params.ParentID = pgtype.UUID{}
		} else {
			parentID := parseUUID(strings.TrimSpace(*req.ParentID))
			if uuidToString(parentID) == projectID {
				writeError(w, http.StatusBadRequest, "project cannot be its own parent")
				return
			}
			if _, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{
				ID:          parentID,
				WorkspaceID: parseUUID(workspaceID),
			}); err != nil {
				writeError(w, http.StatusBadRequest, "parent_id must belong to the workspace")
				return
			}
			params.ParentID = parentID
		}
	}

	updated, err := h.Queries.UpdateProject(r.Context(), params)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "project slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}

	if _, exists := raw["label_ids"]; exists {
		if err := h.setProjectLabels(r, workspaceID, updated.ID, req.LabelIDs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid label_ids")
			return
		}
	}

	resp := projectToResponse(updated)
	if labels, err := h.loadProjectLabelsForProject(r, updated.ID); err == nil {
		resp.Labels = labels
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	projectID := chi.URLParam(r, "projectId")

	project, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{
		ID:          parseUUID(projectID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if project.Kind == "general" {
		writeError(w, http.StatusBadRequest, "default General project cannot be deleted")
		return
	}

	if err := h.Queries.DeleteProject(r.Context(), db.DeleteProjectParams{
		ID:          project.ID,
		WorkspaceID: project.WorkspaceID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete project")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListProjectTree(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	projects, err := h.Queries.ListProjectsByWorkspace(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	nodes := make([]*ProjectTreeNode, 0, len(projects))
	nodeByID := make(map[string]*ProjectTreeNode, len(projects))
	for _, project := range projects {
		resp := projectToResponse(project)
		if labels, err := h.loadProjectLabelsForProject(r, project.ID); err == nil {
			resp.Labels = labels
		}
		node := &ProjectTreeNode{
			Project:  resp,
			Children: []*ProjectTreeNode{},
		}
		nodes = append(nodes, node)
		nodeByID[node.Project.ID] = node
	}

	roots := make([]*ProjectTreeNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Project.ParentID == nil {
			roots = append(roots, node)
			continue
		}
		parent := nodeByID[*node.Project.ParentID]
		if parent == nil {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}

	writeJSON(w, http.StatusOK, roots)
}

type CreateProjectLabelRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

func (h *Handler) ListProjectLabels(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	labels, err := h.Queries.ListProjectLabelsByWorkspace(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list project labels")
		return
	}
	resp := make([]ProjectLabelResponse, 0, len(labels))
	for _, label := range labels {
		resp = append(resp, projectLabelToResponse(label))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateProjectLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	var req CreateProjectLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	color := "blue"
	if req.Color != nil && strings.TrimSpace(*req.Color) != "" {
		color = strings.TrimSpace(*req.Color)
	}
	label, err := h.Queries.CreateProjectLabel(r.Context(), db.CreateProjectLabelParams{
		WorkspaceID: parseUUID(workspaceID),
		Name:        name,
		Color:       color,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "project label already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create project label")
		return
	}
	writeJSON(w, http.StatusCreated, projectLabelToResponse(label))
}

type UpdateProjectLabelRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func (h *Handler) UpdateProjectLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	labelID := chi.URLParam(r, "labelId")
	current, err := h.Queries.GetProjectLabelInWorkspace(r.Context(), db.GetProjectLabelInWorkspaceParams{
		ID:          parseUUID(labelID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "project label not found")
		return
	}

	var req UpdateProjectLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	params := db.UpdateProjectLabelParams{
		ID:          current.ID,
		WorkspaceID: current.WorkspaceID,
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		params.Name = pgtype.Text{String: name, Valid: true}
	}
	if req.Color != nil {
		color := strings.TrimSpace(*req.Color)
		if color == "" {
			writeError(w, http.StatusBadRequest, "color cannot be empty")
			return
		}
		params.Color = pgtype.Text{String: color, Valid: true}
	}

	updated, err := h.Queries.UpdateProjectLabel(r.Context(), params)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "project label already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update project label")
		return
	}
	writeJSON(w, http.StatusOK, projectLabelToResponse(updated))
}

func (h *Handler) DeleteProjectLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	labelID := chi.URLParam(r, "labelId")
	if err := h.Queries.DeleteProjectLabel(r.Context(), db.DeleteProjectLabelParams{
		ID:          parseUUID(labelID),
		WorkspaceID: parseUUID(workspaceID),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete project label")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func valueOrDefault(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}
