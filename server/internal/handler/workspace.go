package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

var nonAlpha = regexp.MustCompile(`[^a-zA-Z]`)

// generateIssuePrefix produces a 2-5 char uppercase prefix from a workspace name.
// Examples: "Jiayuan's Workspace" → "JIA", "My Team" → "MYT", "AB" → "AB".
func generateIssuePrefix(name string) string {
	letters := nonAlpha.ReplaceAllString(name, "")
	if len(letters) == 0 {
		return "WS"
	}
	letters = strings.ToUpper(letters)
	if len(letters) > 3 {
		letters = letters[:3]
	}
	return letters
}

type WorkspaceResponse struct {
	ID            string                          `json:"id"`
	Name          string                          `json:"name"`
	Slug          string                          `json:"slug"`
	Description   *string                         `json:"description"`
	Context       *string                         `json:"context"`
	Settings      any                             `json:"settings"`
	Repos         any                             `json:"repos"`
	KnowledgeRepo *WorkspaceKnowledgeRepoResponse `json:"knowledge_repo,omitempty"`
	IssuePrefix   string                          `json:"issue_prefix"`
	CreatedAt     string                          `json:"created_at"`
	UpdatedAt     string                          `json:"updated_at"`
}

func workspaceToResponse(w db.Workspace) WorkspaceResponse {
	var settings any
	if w.Settings != nil {
		json.Unmarshal(w.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	var repos any
	if w.Repos != nil {
		json.Unmarshal(w.Repos, &repos)
	}
	if repos == nil {
		repos = []any{}
	}
	return WorkspaceResponse{
		ID:          uuidToString(w.ID),
		Name:        w.Name,
		Slug:        w.Slug,
		Description: textToPtr(w.Description),
		Context:     textToPtr(w.Context),
		Settings:    settings,
		Repos:       repos,
		IssuePrefix: w.IssuePrefix,
		CreatedAt:   timestampToString(w.CreatedAt),
		UpdatedAt:   timestampToString(w.UpdatedAt),
	}
}

func (h *Handler) loadKnowledgeRepoResponse(r *http.Request, workspaceID string) *WorkspaceKnowledgeRepoResponse {
	repo, err := h.Queries.GetWorkspaceKnowledgeRepo(r.Context(), parseUUID(workspaceID))
	if err != nil {
		return nil
	}
	resp := knowledgeRepoToResponse(repo)
	return &resp
}

type MemberResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
}

func memberToResponse(m db.Member) MemberResponse {
	return MemberResponse{
		ID:          uuidToString(m.ID),
		WorkspaceID: uuidToString(m.WorkspaceID),
		UserID:      uuidToString(m.UserID),
		Role:        m.Role,
		CreatedAt:   timestampToString(m.CreatedAt),
	}
}

func (h *Handler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	workspaces, err := h.Queries.ListWorkspaces(r.Context(), parseUUID(userID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}

	resp := make([]WorkspaceResponse, len(workspaces))
	for i, ws := range workspaces {
		resp[i] = workspaceToResponse(ws)
		resp[i].KnowledgeRepo = h.loadKnowledgeRepoResponse(r, uuidToString(ws.ID))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	id := workspaceIDFromURL(r, "id")

	ws, err := h.Queries.GetWorkspace(r.Context(), parseUUID(id))
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	resp := workspaceToResponse(ws)
	resp.KnowledgeRepo = h.loadKnowledgeRepoResponse(r, id)
	writeJSON(w, http.StatusOK, resp)
}

type CreateWorkspaceRequest struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	Context     *string `json:"context"`
	IssuePrefix *string `json:"issue_prefix"`
}

func (h *Handler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	var req CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	if req.Name == "" || req.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug are required")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}
	defer tx.Rollback(r.Context())

	issuePrefix := generateIssuePrefix(req.Name)
	if req.IssuePrefix != nil && strings.TrimSpace(*req.IssuePrefix) != "" {
		issuePrefix = strings.ToUpper(strings.TrimSpace(*req.IssuePrefix))
	}

	qtx := h.Queries.WithTx(tx)
	ws, err := qtx.CreateWorkspace(r.Context(), db.CreateWorkspaceParams{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: ptrToText(req.Description),
		Context:     ptrToText(req.Context),
		IssuePrefix: issuePrefix,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "workspace slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create workspace: "+err.Error())
		return
	}

	_, err = qtx.CreateMember(r.Context(), db.CreateMemberParams{
		WorkspaceID: ws.ID,
		UserID:      parseUUID(userID),
		Role:        "owner",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add owner: "+err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}

	slog.Info("workspace created", append(logger.RequestAttrs(r), "workspace_id", uuidToString(ws.ID), "name", ws.Name, "slug", ws.Slug)...)
	resp := workspaceToResponse(ws)
	resp.KnowledgeRepo = h.loadKnowledgeRepoResponse(r, uuidToString(ws.ID))
	writeJSON(w, http.StatusCreated, resp)
}

type UpdateWorkspaceRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Context     *string `json:"context"`
	Settings    any     `json:"settings"`
	Repos       any     `json:"repos"`
	IssuePrefix *string `json:"issue_prefix"`
}

func (h *Handler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	id := workspaceIDFromURL(r, "id")

	var req UpdateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := db.UpdateWorkspaceParams{
		ID: parseUUID(id),
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		params.Name = pgtype.Text{String: name, Valid: true}
	}
	if req.Description != nil {
		params.Description = pgtype.Text{String: *req.Description, Valid: true}
	}
	if req.Context != nil {
		params.Context = pgtype.Text{String: *req.Context, Valid: true}
	}
	if req.Settings != nil {
		s, _ := json.Marshal(req.Settings)
		params.Settings = s
	}
	if req.Repos != nil {
		reposJSON, _ := json.Marshal(req.Repos)
		params.Repos = reposJSON
	}
	if req.IssuePrefix != nil {
		prefix := strings.ToUpper(strings.TrimSpace(*req.IssuePrefix))
		if prefix != "" {
			params.IssuePrefix = pgtype.Text{String: prefix, Valid: true}
		}
	}

	ws, err := h.Queries.UpdateWorkspace(r.Context(), params)
	if err != nil {
		slog.Warn("update workspace failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", id)...)
		writeError(w, http.StatusInternalServerError, "failed to update workspace: "+err.Error())
		return
	}

	slog.Info("workspace updated", append(logger.RequestAttrs(r), "workspace_id", id)...)
	userID := requestUserID(r)
	h.publish(protocol.EventWorkspaceUpdated, id, "member", userID, map[string]any{"workspace": workspaceToResponse(ws)})

	resp := workspaceToResponse(ws)
	resp.KnowledgeRepo = h.loadKnowledgeRepoResponse(r, id)
	writeJSON(w, http.StatusOK, resp)
}

type CreateWorkspaceRepoFromGitHubRequest struct {
	Owner               *string `json:"owner"`
	RepoName            *string `json:"repo_name"`
	Visibility          *string `json:"visibility"`
	Description         *string `json:"description"`
	AddToWorkspaceRepos *bool   `json:"add_to_workspace_repos"`
}

func (h *Handler) CreateWorkspaceRepoFromGitHub(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	if _, err := exec.LookPath("gh"); err != nil {
		writeError(w, http.StatusBadRequest, "gh CLI not found on server host")
		return
	}

	var req CreateWorkspaceRepoFromGitHubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	workspace, err := h.Queries.GetWorkspace(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	owner := strings.TrimSpace(valueOrDefault(req.Owner, ""))
	repoName := normalizeGitHubRepoName(valueOrDefault(req.RepoName, workspace.Slug))
	visibility := strings.ToLower(strings.TrimSpace(valueOrDefault(req.Visibility, "private")))
	description := strings.TrimSpace(valueOrDefault(req.Description, fmt.Sprintf("Repository for %s", workspace.Name)))
	addToWorkspaceRepos := true
	if req.AddToWorkspaceRepos != nil {
		addToWorkspaceRepos = *req.AddToWorkspaceRepos
	}

	switch visibility {
	case "private", "public", "internal":
	default:
		writeError(w, http.StatusBadRequest, "visibility must be one of: private, public, internal")
		return
	}

	fullName := repoName
	if owner != "" {
		fullName = owner + "/" + repoName
	}

	createArgs := []string{
		"repo", "create", fullName,
		"--" + visibility,
		"--description", description,
		"--confirm",
	}
	if out, err := exec.Command("gh", createArgs...).CombinedOutput(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to create GitHub repo via gh: "+trimCommandOutput(out))
		return
	}

	viewOut, err := exec.Command("gh", "repo", "view", fullName, "--json", "nameWithOwner,url,defaultBranchRef").CombinedOutput()
	if err != nil {
		writeError(w, http.StatusBadRequest, "repo created but failed to read metadata from gh: "+trimCommandOutput(viewOut))
		return
	}

	var view ghRepoViewResponse
	if err := json.Unmarshal(viewOut, &view); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse gh repo metadata")
		return
	}
	if strings.TrimSpace(view.URL) == "" {
		writeError(w, http.StatusInternalServerError, "gh returned empty repo url")
		return
	}

	if addToWorkspaceRepos {
		repos := make([]workspaceRepoItem, 0)
		if len(workspace.Repos) > 0 {
			_ = json.Unmarshal(workspace.Repos, &repos)
		}

		exists := false
		for _, item := range repos {
			if strings.EqualFold(strings.TrimSpace(item.URL), strings.TrimSpace(view.URL)) {
				exists = true
				break
			}
		}
		if !exists {
			repos = append(repos, workspaceRepoItem{
				URL:         view.URL,
				Description: description,
			})
			reposJSON, _ := json.Marshal(repos)
			if _, err := h.Queries.UpdateWorkspace(r.Context(), db.UpdateWorkspaceParams{
				ID:    parseUUID(workspaceID),
				Repos: reposJSON,
			}); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to save workspace repositories")
				return
			}
		}
	}

	updatedWorkspace, err := h.Queries.GetWorkspace(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload workspace")
		return
	}
	resp := workspaceToResponse(updatedWorkspace)
	resp.KnowledgeRepo = h.loadKnowledgeRepoResponse(r, workspaceID)

	branch := strings.TrimSpace(view.DefaultBranchRef.Name)
	if branch == "" {
		branch = "main"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"workspace": resp,
		"github_repo": map[string]any{
			"name_with_owner": view.NameWithOwner,
			"url":             view.URL,
			"default_branch":  branch,
			"visibility":      visibility,
		},
	})
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "id")
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}

	members, err := h.Queries.ListMembers(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}

	resp := make([]MemberResponse, len(members))
	for i, m := range members {
		resp[i] = memberToResponse(m)
	}

	writeJSON(w, http.StatusOK, resp)
}

type MemberWithUserResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	UserID      string  `json:"user_id"`
	Role        string  `json:"role"`
	CreatedAt   string  `json:"created_at"`
	Name        string  `json:"name"`
	Email       string  `json:"email"`
	AvatarURL   *string `json:"avatar_url"`
}

func (h *Handler) ListMembersWithUser(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")

	members, err := h.Queries.ListMembersWithUser(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}

	resp := make([]MemberWithUserResponse, len(members))
	for i, m := range members {
		resp[i] = MemberWithUserResponse{
			ID:          uuidToString(m.ID),
			WorkspaceID: uuidToString(m.WorkspaceID),
			UserID:      uuidToString(m.UserID),
			Role:        m.Role,
			CreatedAt:   timestampToString(m.CreatedAt),
			Name:        m.UserName,
			Email:       m.UserEmail,
			AvatarURL:   textToPtr(m.UserAvatarUrl),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type CreateMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func memberWithUserResponse(member db.Member, user db.User) MemberWithUserResponse {
	return MemberWithUserResponse{
		ID:          uuidToString(member.ID),
		WorkspaceID: uuidToString(member.WorkspaceID),
		UserID:      uuidToString(member.UserID),
		Role:        member.Role,
		CreatedAt:   timestampToString(member.CreatedAt),
		Name:        user.Name,
		Email:       user.Email,
		AvatarURL:   textToPtr(user.AvatarUrl),
	}
}

func normalizeMemberRole(role string) (string, bool) {
	if role == "" {
		return "member", true
	}

	role = strings.TrimSpace(role)
	switch role {
	case "owner", "admin", "member":
		return role, true
	default:
		return "", false
	}
}

func (h *Handler) CreateMember(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	requester, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}

	var req CreateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	role, valid := normalizeMemberRole(req.Role)
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid member role")
		return
	}
	if role == "owner" && requester.Role != "owner" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	user, err := h.Queries.GetUserByEmail(r.Context(), email)
	if err != nil {
		if isNotFound(err) {
			// Auto-create user with email so they can be invited before signing up
			user, err = h.Queries.CreateUser(r.Context(), db.CreateUserParams{
				Name:  email,
				Email: email,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to create user")
				return
			}
		} else {
			writeError(w, http.StatusInternalServerError, "failed to load user")
			return
		}
	}

	member, err := h.Queries.CreateMember(r.Context(), db.CreateMemberParams{
		WorkspaceID: parseUUID(workspaceID),
		UserID:      user.ID,
		Role:        role,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "user is already a member")
			return
		}
		slog.Warn("create member failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID, "email", email)...)
		writeError(w, http.StatusInternalServerError, "failed to create member")
		return
	}

	slog.Info("member added", append(logger.RequestAttrs(r), "member_id", uuidToString(member.ID), "workspace_id", workspaceID, "email", email, "role", role)...)
	userID := requestUserID(r)
	eventPayload := map[string]any{"member": memberWithUserResponse(member, user)}
	if ws, err := h.Queries.GetWorkspace(r.Context(), parseUUID(workspaceID)); err == nil {
		eventPayload["workspace_name"] = ws.Name
	}
	h.publish(protocol.EventMemberAdded, workspaceID, "member", userID, eventPayload)

	writeJSON(w, http.StatusCreated, memberWithUserResponse(member, user))
}

type UpdateMemberRequest struct {
	Role string `json:"role"`
}

func (h *Handler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	requester, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}

	memberID := chi.URLParam(r, "memberId")
	target, err := h.Queries.GetMember(r.Context(), parseUUID(memberID))
	if err != nil || uuidToString(target.WorkspaceID) != workspaceID {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}

	var req UpdateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Role) == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}

	role, valid := normalizeMemberRole(req.Role)
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid member role")
		return
	}

	if (target.Role == "owner" || role == "owner") && requester.Role != "owner" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	if target.Role == "owner" && role != "owner" {
		members, err := h.Queries.ListMembers(r.Context(), target.WorkspaceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update member")
			return
		}
		if countOwners(members) <= 1 {
			writeError(w, http.StatusBadRequest, "workspace must have at least one owner")
			return
		}
	}

	updatedMember, err := h.Queries.UpdateMemberRole(r.Context(), db.UpdateMemberRoleParams{
		ID:   target.ID,
		Role: role,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update member")
		return
	}

	user, err := h.Queries.GetUser(r.Context(), updatedMember.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load member")
		return
	}

	userID := requestUserID(r)
	h.publish(protocol.EventMemberUpdated, workspaceID, "member", userID, map[string]any{
		"member": memberWithUserResponse(updatedMember, user),
	})

	writeJSON(w, http.StatusOK, memberWithUserResponse(updatedMember, user))
}

func (h *Handler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	requester, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}

	memberID := chi.URLParam(r, "memberId")
	target, err := h.Queries.GetMember(r.Context(), parseUUID(memberID))
	if err != nil || uuidToString(target.WorkspaceID) != workspaceID {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}

	if target.Role == "owner" && requester.Role != "owner" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	if target.Role == "owner" {
		members, err := h.Queries.ListMembers(r.Context(), target.WorkspaceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete member")
			return
		}
		if countOwners(members) <= 1 {
			writeError(w, http.StatusBadRequest, "workspace must have at least one owner")
			return
		}
	}

	if err := h.Queries.DeleteMember(r.Context(), target.ID); err != nil {
		slog.Warn("delete member failed", append(logger.RequestAttrs(r), "error", err, "member_id", memberID, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to delete member")
		return
	}

	slog.Info("member removed", append(logger.RequestAttrs(r), "member_id", uuidToString(target.ID), "workspace_id", workspaceID, "user_id", uuidToString(target.UserID))...)
	userID := requestUserID(r)
	h.publish(protocol.EventMemberRemoved, workspaceID, "member", userID, map[string]any{
		"member_id":    uuidToString(target.ID),
		"workspace_id": workspaceID,
		"user_id":      uuidToString(target.UserID),
	})

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) LeaveWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}

	if member.Role == "owner" {
		members, err := h.Queries.ListMembers(r.Context(), member.WorkspaceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to leave workspace")
			return
		}
		if countOwners(members) <= 1 {
			writeError(w, http.StatusBadRequest, "workspace must have at least one owner")
			return
		}
	}

	if err := h.Queries.DeleteMember(r.Context(), member.ID); err != nil {
		slog.Warn("leave workspace failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to leave workspace")
		return
	}

	slog.Info("member removed", append(logger.RequestAttrs(r), "member_id", uuidToString(member.ID), "workspace_id", workspaceID, "user_id", uuidToString(member.UserID))...)
	userID := requestUserID(r)
	h.publish(protocol.EventMemberRemoved, workspaceID, "member", userID, map[string]any{
		"member_id":    uuidToString(member.ID),
		"workspace_id": workspaceID,
		"user_id":      uuidToString(member.UserID),
	})

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")

	if err := h.Queries.DeleteWorkspace(r.Context(), parseUUID(workspaceID)); err != nil {
		slog.Warn("delete workspace failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to delete workspace")
		return
	}

	slog.Info("workspace deleted", append(logger.RequestAttrs(r), "workspace_id", workspaceID)...)
	h.publish(protocol.EventWorkspaceDeleted, workspaceID, "member", requestUserID(r), map[string]any{
		"workspace_id": workspaceID,
	})

	w.WriteHeader(http.StatusNoContent)
}
