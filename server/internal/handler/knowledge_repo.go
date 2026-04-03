package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
	repoConfig, err := h.getOrInitKnowledgeRepo(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load knowledge repo config")
		return
	}

	entries := knowledge.HarnessTemplate()
	if strings.TrimSpace(repoConfig.RepoUrl) != "" {
		if _, err := exec.LookPath("gh"); err != nil {
			writeError(w, http.StatusBadRequest, "gh CLI not found on server host")
			return
		}
		if err := writeHarnessTemplateToRepo(r.Context(), repoConfig.RepoUrl, repoConfig.DefaultBranch, entries); err != nil {
			writeError(w, http.StatusBadRequest, "failed to initialize knowledge repository: "+err.Error())
			return
		}
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
		"entries":          entries,
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

type ghRepoContentResponse struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Size        int64  `json:"size"`
	SHA         string `json:"sha"`
	Content     string `json:"content"`
	Encoding    string `json:"encoding"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
}

type KnowledgeRepoEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Size        int64  `json:"size"`
	SHA         string `json:"sha"`
	HTMLURL     string `json:"html_url,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

type ListWorkspaceKnowledgeRepoContentsResponse struct {
	Path          string               `json:"path"`
	DefaultBranch string               `json:"default_branch"`
	Entries       []KnowledgeRepoEntry `json:"entries"`
}

type GetWorkspaceKnowledgeRepoFileResponse struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	SHA         string `json:"sha"`
	Size        int64  `json:"size"`
	Encoding    string `json:"encoding"`
	Content     string `json:"content"`
	HTMLURL     string `json:"html_url,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

type UpsertWorkspaceKnowledgeRepoFileRequest struct {
	Path    string  `json:"path"`
	Content string  `json:"content"`
	Message *string `json:"message"`
	SHA     *string `json:"sha"`
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

func writeHarnessTemplateToRepo(ctx context.Context, repoURL string, defaultBranch string, entries []knowledge.TemplateEntry) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git CLI not found on server host")
	}

	owner, repo, err := parseGitHubRepoURL(repoURL)
	if err != nil {
		return err
	}
	repoName := owner + "/" + repo

	tempDir, err := os.MkdirTemp("", "multica-knowledge-bootstrap-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory")
	}
	defer os.RemoveAll(tempDir)

	if out, err := exec.CommandContext(ctx, "gh", "repo", "clone", repoName, tempDir, "--", "-q").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to clone knowledge repository: %s", trimCommandOutput(out))
	}

	branch := strings.TrimSpace(defaultBranch)
	if branch == "" {
		branch = "main"
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", tempDir, "checkout", "-B", branch).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout branch %q: %s", branch, trimCommandOutput(out))
	}

	for _, entry := range entries {
		cleanPath := strings.TrimSpace(filepath.Clean(entry.Path))
		if cleanPath == "" || cleanPath == "." || cleanPath == string(filepath.Separator) {
			continue
		}
		if strings.HasPrefix(cleanPath, "..") || filepath.IsAbs(cleanPath) {
			return fmt.Errorf("unsafe template path: %s", entry.Path)
		}

		target := filepath.Join(tempDir, cleanPath)
		if entry.Type == knowledge.EntryTypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %s", cleanPath)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("failed to create parent directory for %s", cleanPath)
		}
		if err := os.WriteFile(target, []byte(entry.Content), 0o644); err != nil {
			return fmt.Errorf("failed to write file %s", cleanPath)
		}
	}

	if out, err := exec.CommandContext(ctx, "git", "-C", tempDir, "add", ".").CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s", trimCommandOutput(out))
	}

	diffErr := exec.CommandContext(ctx, "git", "-C", tempDir, "diff", "--cached", "--quiet").Run()
	if diffErr == nil {
		return nil
	}

	if out, err := exec.CommandContext(
		ctx,
		"git",
		"-C", tempDir,
		"-c", "user.name=multica-bot",
		"-c", "user.email=bot@multica.local",
		"commit",
		"-m", "docs: bootstrap harness knowledge skeleton",
	).CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %s", trimCommandOutput(out))
	}

	if out, err := exec.CommandContext(ctx, "git", "-C", tempDir, "push", "-u", "origin", branch).CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %s", trimCommandOutput(out))
	}

	return nil
}

func parseGitHubRepoURL(raw string) (owner string, repo string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("knowledge repository URL is not configured")
	}

	if strings.HasPrefix(trimmed, "git@github.com:") {
		trimmed = "https://github.com/" + strings.TrimPrefix(trimmed, "git@github.com:")
	} else if strings.HasPrefix(trimmed, "github.com/") {
		trimmed = "https://" + trimmed
	}

	parsed, parseErr := neturl.Parse(trimmed)
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid knowledge repository URL")
	}
	if !strings.EqualFold(parsed.Hostname(), "github.com") {
		return "", "", fmt.Errorf("knowledge repository must be hosted on github.com")
	}

	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("knowledge repository URL must include owner and repo")
	}

	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("knowledge repository URL must include owner and repo")
	}

	return owner, repo, nil
}

func buildGitHubContentsAPIPath(owner string, repo string, filePath string, ref string) string {
	base := fmt.Sprintf("repos/%s/%s/contents", owner, repo)
	cleanPath := strings.Trim(strings.TrimSpace(filePath), "/")
	if cleanPath != "" {
		segments := strings.Split(cleanPath, "/")
		escaped := make([]string, 0, len(segments))
		for _, segment := range segments {
			part := strings.TrimSpace(segment)
			if part == "" {
				continue
			}
			escaped = append(escaped, neturl.PathEscape(part))
		}
		if len(escaped) > 0 {
			base += "/" + strings.Join(escaped, "/")
		}
	}
	if strings.TrimSpace(ref) != "" {
		base += "?ref=" + neturl.QueryEscape(strings.TrimSpace(ref))
	}
	return base
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

	entries := knowledge.HarnessTemplate()
	if err := writeHarnessTemplateToRepo(r.Context(), view.URL, branch, entries); err != nil {
		writeError(w, http.StatusInternalServerError, "repository created but failed to bootstrap harness template: "+err.Error())
		return
	}

	_, err = h.Queries.UpsertWorkspaceKnowledgeRepo(r.Context(), db.UpsertWorkspaceKnowledgeRepoParams{
		WorkspaceID:        parseUUID(workspaceID),
		RepoUrl:            view.URL,
		DefaultBranch:      branch,
		CuratorAgentID:     repoConfig.CuratorAgentID,
		TemplateVersion:    knowledge.TemplateVersion,
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

	bootstrappedRepo, err := h.Queries.MarkKnowledgeRepoBootstrapped(r.Context(), db.MarkKnowledgeRepoBootstrappedParams{
		WorkspaceID:     parseUUID(workspaceID),
		TemplateVersion: knowledge.TemplateVersion,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark knowledge repo as bootstrapped")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"knowledge_repo": knowledgeRepoToResponse(bootstrappedRepo),
		"github_repo": map[string]any{
			"name_with_owner": view.NameWithOwner,
			"url":             view.URL,
			"default_branch":  branch,
			"visibility":      visibility,
		},
		"template_version": knowledge.TemplateVersion,
		"entries":          entries,
	})
}

func (h *Handler) ListWorkspaceKnowledgeRepoContents(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	if _, err := exec.LookPath("gh"); err != nil {
		writeError(w, http.StatusBadRequest, "gh CLI not found on server host")
		return
	}

	repoConfig, err := h.getOrInitKnowledgeRepo(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load knowledge repo config")
		return
	}
	owner, repo, err := parseGitHubRepoURL(repoConfig.RepoUrl)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	requestPath := strings.Trim(strings.TrimSpace(r.URL.Query().Get("path")), "/")
	apiPath := buildGitHubContentsAPIPath(owner, repo, requestPath, repoConfig.DefaultBranch)
	out, cmdErr := exec.Command("gh", "api", "-H", "Accept: application/vnd.github+json", apiPath).CombinedOutput()
	if cmdErr != nil {
		if requestPath == "" && isGitHubEmptyRepoError(out) {
			writeJSON(w, http.StatusOK, ListWorkspaceKnowledgeRepoContentsResponse{
				Path:          requestPath,
				DefaultBranch: repoConfig.DefaultBranch,
				Entries:       []KnowledgeRepoEntry{},
			})
			return
		}
		writeError(w, http.StatusBadRequest, "failed to list knowledge repository contents: "+trimCommandOutput(out))
		return
	}

	trimmed := strings.TrimSpace(string(out))
	if strings.HasPrefix(trimmed, "{") {
		var file ghRepoContentResponse
		if err := json.Unmarshal(out, &file); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse knowledge repository contents")
			return
		}
		if file.Type == "file" {
			writeError(w, http.StatusBadRequest, "path points to a file; use the file endpoint")
			return
		}
		writeError(w, http.StatusInternalServerError, "unexpected GitHub response shape")
		return
	}

	var items []ghRepoContentResponse
	if err := json.Unmarshal(out, &items); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse knowledge repository contents")
		return
	}

	entries := make([]KnowledgeRepoEntry, 0, len(items))
	for _, item := range items {
		if item.Type != "dir" && item.Type != "file" {
			continue
		}
		entries = append(entries, KnowledgeRepoEntry{
			Name:        item.Name,
			Path:        item.Path,
			Type:        item.Type,
			Size:        item.Size,
			SHA:         item.SHA,
			HTMLURL:     item.HTMLURL,
			DownloadURL: item.DownloadURL,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "dir"
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	writeJSON(w, http.StatusOK, ListWorkspaceKnowledgeRepoContentsResponse{
		Path:          requestPath,
		DefaultBranch: repoConfig.DefaultBranch,
		Entries:       entries,
	})
}

func isGitHubEmptyRepoError(output []byte) bool {
	normalized := strings.ToLower(strings.TrimSpace(string(output)))
	return strings.Contains(normalized, "this repository is empty")
}

func (h *Handler) GetWorkspaceKnowledgeRepoFile(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	if _, err := exec.LookPath("gh"); err != nil {
		writeError(w, http.StatusBadRequest, "gh CLI not found on server host")
		return
	}

	repoPath := strings.Trim(strings.TrimSpace(r.URL.Query().Get("path")), "/")
	if repoPath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	repoConfig, err := h.getOrInitKnowledgeRepo(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load knowledge repo config")
		return
	}
	owner, repo, err := parseGitHubRepoURL(repoConfig.RepoUrl)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	apiPath := buildGitHubContentsAPIPath(owner, repo, repoPath, repoConfig.DefaultBranch)
	out, cmdErr := exec.Command("gh", "api", "-H", "Accept: application/vnd.github+json", apiPath).CombinedOutput()
	if cmdErr != nil {
		writeError(w, http.StatusBadRequest, "failed to load knowledge repository file: "+trimCommandOutput(out))
		return
	}

	var file ghRepoContentResponse
	if err := json.Unmarshal(out, &file); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse knowledge repository file")
		return
	}
	if file.Type != "file" {
		writeError(w, http.StatusBadRequest, "path is not a file")
		return
	}

	content := strings.ReplaceAll(file.Content, "\n", "")
	decodedContent := ""
	if strings.EqualFold(strings.TrimSpace(file.Encoding), "base64") {
		decoded, decodeErr := base64.StdEncoding.DecodeString(content)
		if decodeErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to decode file content")
			return
		}
		decodedContent = string(decoded)
	} else {
		decodedContent = content
	}

	writeJSON(w, http.StatusOK, GetWorkspaceKnowledgeRepoFileResponse{
		Path:        file.Path,
		Name:        file.Name,
		SHA:         file.SHA,
		Size:        file.Size,
		Encoding:    file.Encoding,
		Content:     decodedContent,
		HTMLURL:     file.HTMLURL,
		DownloadURL: file.DownloadURL,
	})
}

func (h *Handler) UpsertWorkspaceKnowledgeRepoFile(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	if _, err := exec.LookPath("gh"); err != nil {
		writeError(w, http.StatusBadRequest, "gh CLI not found on server host")
		return
	}

	var req UpsertWorkspaceKnowledgeRepoFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	repoPath := strings.Trim(strings.TrimSpace(req.Path), "/")
	if repoPath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	repoConfig, err := h.getOrInitKnowledgeRepo(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load knowledge repo config")
		return
	}
	owner, repo, err := parseGitHubRepoURL(repoConfig.RepoUrl)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	message := strings.TrimSpace(valueOrDefault(req.Message, "docs: update knowledge repository file"))
	contentBase64 := base64.StdEncoding.EncodeToString([]byte(req.Content))
	apiPath := buildGitHubContentsAPIPath(owner, repo, repoPath, "")
	args := []string{
		"api",
		"--method", "PUT",
		"-H", "Accept: application/vnd.github+json",
		apiPath,
		"-f", "message=" + message,
		"-f", "content=" + contentBase64,
		"-f", "branch=" + repoConfig.DefaultBranch,
	}
	if req.SHA != nil && strings.TrimSpace(*req.SHA) != "" {
		args = append(args, "-f", "sha="+strings.TrimSpace(*req.SHA))
	}

	out, cmdErr := exec.Command("gh", args...).CombinedOutput()
	if cmdErr != nil {
		writeError(w, http.StatusBadRequest, "failed to save knowledge repository file: "+trimCommandOutput(out))
		return
	}

	var result struct {
		Content struct {
			Path string `json:"path"`
			SHA  string `json:"sha"`
			Size int64  `json:"size"`
		} `json:"content"`
		Commit struct {
			HTMLURL string `json:"html_url"`
			SHA     string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse knowledge repository update result")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":           result.Content.Path,
		"sha":            result.Content.SHA,
		"size":           result.Content.Size,
		"commit_sha":     result.Commit.SHA,
		"commit_url":     result.Commit.HTMLURL,
		"default_branch": repoConfig.DefaultBranch,
	})
}
