package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/knowledge"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var githubRepoNameSanitizer = regexp.MustCompile(`[^a-z0-9._-]+`)

type WorkspaceKnowledgeRepoResponse struct {
	WorkspaceID        string  `json:"workspace_id"`
	RepoURL            string  `json:"repo_url"`
	DefaultBranch      string  `json:"default_branch"`
	CuratorAgentID     *string `json:"curator_agent_id"`
	TemplateVersion    string  `json:"template_version"`
	Mode               string  `json:"mode"`
	Enabled            bool    `json:"enabled"`
	LastBootstrappedAt *string `json:"last_bootstrapped_at"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

func knowledgeRepoToResponse(repo db.WorkspaceKnowledgeRepo) WorkspaceKnowledgeRepoResponse {
	return WorkspaceKnowledgeRepoResponse{
		WorkspaceID:        uuidToString(repo.WorkspaceID),
		RepoURL:            repo.RepoUrl,
		DefaultBranch:      repo.DefaultBranch,
		CuratorAgentID:     uuidToPtr(repo.CuratorAgentID),
		TemplateVersion:    repo.TemplateVersion,
		Mode:               repo.Mode,
		Enabled:            repo.Enabled,
		LastBootstrappedAt: timestampToPtr(repo.LastBootstrappedAt),
		CreatedAt:          timestampToString(repo.CreatedAt),
		UpdatedAt:          timestampToString(repo.UpdatedAt),
	}
}

func (h *Handler) getOrInitKnowledgeRepo(r *http.Request, workspaceID string) (db.WorkspaceKnowledgeRepo, error) {
	repo, err := h.Queries.GetWorkspaceKnowledgeRepo(r.Context(), parseUUID(workspaceID))
	if err == nil {
		return repo, nil
	}
	return h.Queries.UpsertWorkspaceKnowledgeRepo(r.Context(), db.UpsertWorkspaceKnowledgeRepoParams{
		WorkspaceID:        parseUUID(workspaceID),
		RepoUrl:            "",
		DefaultBranch:      "main",
		CuratorAgentID:     pgtype.UUID{},
		TemplateVersion:    knowledge.TemplateVersion,
		Mode:               "pr",
		Enabled:            true,
		LastBootstrappedAt: pgtype.Timestamptz{},
	})
}

func (h *Handler) GetWorkspaceKnowledgeRepo(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	repo, err := h.getOrInitKnowledgeRepo(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load knowledge repo config")
		return
	}
	writeJSON(w, http.StatusOK, knowledgeRepoToResponse(repo))
}

type UpdateWorkspaceKnowledgeRepoRequest struct {
	RepoURL         *string `json:"repo_url"`
	DefaultBranch   *string `json:"default_branch"`
	CuratorAgentID  *string `json:"curator_agent_id"`
	TemplateVersion *string `json:"template_version"`
	Mode            *string `json:"mode"`
	Enabled         *bool   `json:"enabled"`
}

func (h *Handler) UpdateWorkspaceKnowledgeRepo(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var req UpdateWorkspaceKnowledgeRepoRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var raw map[string]json.RawMessage
	_ = json.Unmarshal(bodyBytes, &raw)

	current, err := h.getOrInitKnowledgeRepo(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load knowledge repo config")
		return
	}

	params := db.UpsertWorkspaceKnowledgeRepoParams{
		WorkspaceID:        parseUUID(workspaceID),
		RepoUrl:            current.RepoUrl,
		DefaultBranch:      current.DefaultBranch,
		CuratorAgentID:     current.CuratorAgentID,
		TemplateVersion:    current.TemplateVersion,
		Mode:               current.Mode,
		Enabled:            current.Enabled,
		LastBootstrappedAt: current.LastBootstrappedAt,
	}

	if req.RepoURL != nil {
		params.RepoUrl = strings.TrimSpace(*req.RepoURL)
	}
	if req.DefaultBranch != nil {
		branch := strings.TrimSpace(*req.DefaultBranch)
		if branch == "" {
			writeError(w, http.StatusBadRequest, "default_branch cannot be empty")
			return
		}
		params.DefaultBranch = branch
	}
	if req.TemplateVersion != nil {
		v := strings.TrimSpace(*req.TemplateVersion)
		if v != "" {
			params.TemplateVersion = v
		}
	}
	if req.Mode != nil {
		mode := strings.TrimSpace(*req.Mode)
		if mode == "" {
			writeError(w, http.StatusBadRequest, "mode cannot be empty")
			return
		}
		if mode != "pr" {
			writeError(w, http.StatusBadRequest, "unsupported mode, only 'pr' is allowed")
			return
		}
		params.Mode = mode
	}
	if req.Enabled != nil {
		params.Enabled = *req.Enabled
	}

	if _, exists := raw["curator_agent_id"]; exists {
		if req.CuratorAgentID == nil || strings.TrimSpace(*req.CuratorAgentID) == "" {
			params.CuratorAgentID = pgtype.UUID{}
		} else {
			curatorID := strings.TrimSpace(*req.CuratorAgentID)
			agent, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
				ID:          parseUUID(curatorID),
				WorkspaceID: parseUUID(workspaceID),
			})
			if err != nil {
				writeError(w, http.StatusBadRequest, "curator_agent_id must belong to the workspace")
				return
			}
			params.CuratorAgentID = agent.ID
		}
	}

	repo, err := h.Queries.UpsertWorkspaceKnowledgeRepo(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update knowledge repo config")
		return
	}

	writeJSON(w, http.StatusOK, knowledgeRepoToResponse(repo))
}

type KnowledgeRepoBootstrapResponse struct {
	TemplateVersion string                    `json:"template_version"`
	Entries         []knowledge.TemplateEntry `json:"entries"`
}

func (h *Handler) BootstrapWorkspaceKnowledgeRepo(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	if _, err := h.getOrInitKnowledgeRepo(r, workspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load knowledge repo config")
		return
	}
	repo, err := h.Queries.MarkKnowledgeRepoBootstrapped(r.Context(), db.MarkKnowledgeRepoBootstrappedParams{
		WorkspaceID:     parseUUID(workspaceID),
		TemplateVersion: knowledge.TemplateVersion,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark knowledge repo as bootstrapped")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"knowledge_repo":   knowledgeRepoToResponse(repo),
		"template_version": knowledge.TemplateVersion,
		"entries":          knowledge.HarnessTemplate(),
	})
}

type ValidateWorkspaceKnowledgeRepoRequest struct {
	Entries []knowledge.TemplateEntry `json:"entries"`
}

func (h *Handler) ValidateWorkspaceKnowledgeRepo(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	if _, err := h.getOrInitKnowledgeRepo(r, workspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load knowledge repo config")
		return
	}

	var req ValidateWorkspaceKnowledgeRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Entries) == 0 {
		writeError(w, http.StatusBadRequest, "entries are required")
		return
	}

	result := knowledge.Validate(req.Entries)
	writeJSON(w, http.StatusOK, result)
}

type CreateKnowledgeRepoFromGitHubRequest struct {
	Owner               *string `json:"owner"`
	RepoName            *string `json:"repo_name"`
	Visibility          *string `json:"visibility"`
	Description         *string `json:"description"`
	DefaultBranch       *string `json:"default_branch"`
	AddToWorkspaceRepos *bool   `json:"add_to_workspace_repos"`
}

type ghRepoViewResponse struct {
	NameWithOwner    string `json:"nameWithOwner"`
	URL              string `json:"url"`
	DefaultBranchRef struct {
		Name string `json:"name"`
	} `json:"defaultBranchRef"`
}

type workspaceRepoItem struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

func normalizeGitHubRepoName(v string) string {
	name := strings.ToLower(strings.TrimSpace(v))
	name = githubRepoNameSanitizer.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		return "knowledge-base"
	}
	return name
}

func trimCommandOutput(out []byte) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return ""
	}
	const maxLen = 300
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

func (h *Handler) CreateWorkspaceKnowledgeRepoFromGitHub(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	if _, err := exec.LookPath("gh"); err != nil {
		writeError(w, http.StatusBadRequest, "gh CLI not found on server host")
		return
	}

	var req CreateKnowledgeRepoFromGitHubRequest
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
	repoName := normalizeGitHubRepoName(valueOrDefault(req.RepoName, workspace.Slug+"-knowledge"))
	visibility := strings.ToLower(strings.TrimSpace(valueOrDefault(req.Visibility, "private")))
	description := strings.TrimSpace(valueOrDefault(req.Description, fmt.Sprintf("Knowledge repository for %s", workspace.Name)))
	defaultBranch := strings.TrimSpace(valueOrDefault(req.DefaultBranch, ""))
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

	repoConfig, err := h.getOrInitKnowledgeRepo(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize knowledge repo config")
		return
	}

	branch := strings.TrimSpace(view.DefaultBranchRef.Name)
	if defaultBranch != "" {
		branch = defaultBranch
	}
	if branch == "" {
		branch = "main"
	}

	updatedRepo, err := h.Queries.UpsertWorkspaceKnowledgeRepo(r.Context(), db.UpsertWorkspaceKnowledgeRepoParams{
		WorkspaceID:        parseUUID(workspaceID),
		RepoUrl:            view.URL,
		DefaultBranch:      branch,
		CuratorAgentID:     repoConfig.CuratorAgentID,
		TemplateVersion:    repoConfig.TemplateVersion,
		Mode:               repoConfig.Mode,
		Enabled:            repoConfig.Enabled,
		LastBootstrappedAt: repoConfig.LastBootstrappedAt,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save knowledge repo config")
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
				Description: "Knowledge repository",
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

	writeJSON(w, http.StatusOK, map[string]any{
		"knowledge_repo": knowledgeRepoToResponse(updatedRepo),
		"github_repo": map[string]any{
			"name_with_owner": view.NameWithOwner,
			"url":             view.URL,
			"default_branch":  branch,
			"visibility":      visibility,
		},
	})
}
