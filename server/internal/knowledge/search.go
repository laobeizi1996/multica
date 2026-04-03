package knowledge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultLookupTopK = 8
	maxQueryTerms     = 20
	maxSnippetLen     = 220
)

type LookupInput struct {
	WorkspaceID   string
	IssueID       string
	RepoURL       string
	DefaultBranch string
	QueryTexts    []string
	RepoDir       string
	TopK          int
}

type KnowledgeRetriever interface {
	LookupKnowledge(ctx context.Context, input LookupInput) (LookupResult, error)
}

type DefaultRetriever struct{}

type KnowledgeMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
	Score   int    `json:"score"`
	Reason  string `json:"reason"`
}

type LookupResult struct {
	WorkspaceID   string           `json:"workspace_id"`
	IssueID       string           `json:"issue_id"`
	RepoURL       string           `json:"repo_url"`
	DefaultBranch string           `json:"default_branch"`
	QueryTerms    []string         `json:"query_terms"`
	Matches       []KnowledgeMatch `json:"matches"`
	HitCount      int              `json:"hit_count"`
	NoMatchReason string           `json:"no_match_reason,omitempty"`
	DurationMS    int64            `json:"duration_ms"`
	LookedUpAt    string           `json:"looked_up_at"`
}

var (
	queryTermPattern = regexp.MustCompile(`[\p{Han}]{2,}|[a-zA-Z0-9][a-zA-Z0-9_\-/.]{2,}`)
)

var stopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "that": {}, "this": {}, "are": {},
	"was": {}, "were": {}, "have": {}, "has": {}, "had": {}, "will": {}, "would": {}, "could": {},
	"should": {}, "can": {}, "cannot": {}, "into": {}, "onto": {}, "about": {}, "after": {},
	"before": {}, "when": {}, "where": {}, "which": {}, "what": {}, "why": {}, "how": {}, "you": {},
	"your": {}, "our": {}, "ours": {}, "they": {}, "them": {}, "their": {}, "agent": {},
	"issue": {}, "task": {}, "todo": {}, "done": {}, "fix": {}, "update": {}, "add": {}, "new": {},
}

func LookupKnowledge(ctx context.Context, input LookupInput) (LookupResult, error) {
	start := time.Now()
	result := LookupResult{
		WorkspaceID:   strings.TrimSpace(input.WorkspaceID),
		IssueID:       strings.TrimSpace(input.IssueID),
		RepoURL:       strings.TrimSpace(input.RepoURL),
		DefaultBranch: normalizeBranch(input.DefaultBranch),
		LookedUpAt:    start.UTC().Format(time.RFC3339),
	}
	defer func() {
		result.DurationMS = time.Since(start).Milliseconds()
	}()

	if result.RepoURL == "" {
		return result, fmt.Errorf("knowledge repository URL is not configured")
	}

	if _, err := exec.LookPath("git"); err != nil {
		return result, fmt.Errorf("git CLI not found on host")
	}
	if _, err := exec.LookPath("rg"); err != nil {
		return result, fmt.Errorf("rg not found on host")
	}

	repoDir := strings.TrimSpace(input.RepoDir)
	if repoDir == "" {
		return result, fmt.Errorf("lookup repo dir is required")
	}
	if err := syncLookupRepo(ctx, repoDir, result.RepoURL, result.DefaultBranch); err != nil {
		return result, err
	}

	topK := input.TopK
	if topK <= 0 {
		topK = DefaultLookupTopK
	}

	terms := extractQueryTerms(input.QueryTexts)
	result.QueryTerms = terms
	if len(terms) == 0 {
		result.NoMatchReason = "no query terms extracted from issue context"
		return result, nil
	}

	matches := make([]KnowledgeMatch, 0, topK*2)
	matches = append(matches, scanPriorityFiles(repoDir, terms)...)

	fulltextMatches, err := scanDocsByRipgrep(ctx, repoDir, terms)
	if err != nil {
		return result, err
	}
	matches = append(matches, fulltextMatches...)

	matches = dedupeAndSortMatches(matches)
	if len(matches) > topK {
		matches = matches[:topK]
	}
	result.Matches = matches
	result.HitCount = len(matches)
	if result.HitCount == 0 {
		result.NoMatchReason = "lookup completed but no relevant knowledge snippets were found"
	}

	return result, nil
}

func (DefaultRetriever) LookupKnowledge(ctx context.Context, input LookupInput) (LookupResult, error) {
	return LookupKnowledge(ctx, input)
}

func normalizeBranch(v string) string {
	branch := strings.TrimSpace(v)
	if branch == "" {
		return "main"
	}
	return branch
}

func syncLookupRepo(ctx context.Context, repoDir string, repoURL string, branch string) error {
	if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
		return fmt.Errorf("failed to create lookup repo parent directory: %w", err)
	}

	gitDir := filepath.Join(repoDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		if out, runErr := exec.CommandContext(ctx, "git", "-C", repoDir, "remote", "set-url", "origin", repoURL).CombinedOutput(); runErr != nil {
			return fmt.Errorf("failed to set knowledge repo remote: %s", trimOutput(out))
		}
		if out, runErr := exec.CommandContext(ctx, "git", "-C", repoDir, "fetch", "--depth", "1", "origin", branch).CombinedOutput(); runErr != nil {
			return fmt.Errorf("failed to fetch knowledge repo: %s", trimOutput(out))
		}
		if out, runErr := exec.CommandContext(ctx, "git", "-C", repoDir, "checkout", "-B", branch, "FETCH_HEAD").CombinedOutput(); runErr != nil {
			return fmt.Errorf("failed to checkout knowledge repo branch: %s", trimOutput(out))
		}
		return nil
	}

	if _, err := os.Stat(repoDir); err == nil {
		if removeErr := os.RemoveAll(repoDir); removeErr != nil {
			return fmt.Errorf("failed to clean lookup repo directory: %w", removeErr)
		}
	}

	if out, runErr := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", branch, repoURL, repoDir).CombinedOutput(); runErr != nil {
		return fmt.Errorf("failed to clone knowledge repo: %s", trimOutput(out))
	}
	return nil
}

func extractQueryTerms(texts []string) []string {
	seen := make(map[string]struct{}, maxQueryTerms)
	terms := make([]string, 0, maxQueryTerms)

	for _, text := range texts {
		for _, token := range queryTermPattern.FindAllString(text, -1) {
			normalized := strings.ToLower(strings.TrimSpace(token))
			if normalized == "" {
				continue
			}
			if _, blocked := stopWords[normalized]; blocked {
				continue
			}
			if utf8.RuneCountInString(normalized) < 2 {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			terms = append(terms, normalized)
			if len(terms) >= maxQueryTerms {
				return terms
			}
		}
	}

	return terms
}

func scanPriorityFiles(repoDir string, terms []string) []KnowledgeMatch {
	priorityFiles := []string{
		"AGENTS.md",
		"ARCHITECTURE.md",
		"docs/README.md",
	}

	indexFiles, _ := filepath.Glob(filepath.Join(repoDir, "docs", "*", "index.md"))
	for _, abs := range indexFiles {
		rel, err := filepath.Rel(repoDir, abs)
		if err != nil {
			continue
		}
		priorityFiles = append(priorityFiles, filepath.ToSlash(rel))
	}

	matches := make([]KnowledgeMatch, 0, len(priorityFiles))
	for _, relPath := range priorityFiles {
		content, err := os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(relPath)))
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(content))
		bestTerm := ""
		bestLine := 1
		termHits := 0

		lines := strings.Split(string(content), "\n")
		for _, term := range terms {
			idx := strings.Index(lower, term)
			if idx < 0 {
				continue
			}
			termHits++
			if bestTerm == "" {
				bestTerm = term
			}
		}
		if termHits == 0 {
			continue
		}

		snippet := ""
		for i, line := range lines {
			lineLower := strings.ToLower(line)
			if bestTerm != "" && strings.Contains(lineLower, bestTerm) {
				bestLine = i + 1
				snippet = line
				break
			}
		}
		if strings.TrimSpace(snippet) == "" {
			snippet = strings.TrimSpace(lines[0])
		}

		baseScore := 90
		if relPath == "AGENTS.md" || relPath == "ARCHITECTURE.md" || relPath == "docs/README.md" {
			baseScore = 100
		}
		matches = append(matches, KnowledgeMatch{
			Path:    relPath,
			Line:    bestLine,
			Snippet: trimSnippet(snippet),
			Score:   baseScore + termHits*5,
			Reason:  "priority-index",
		})
	}

	return matches
}

func scanDocsByRipgrep(ctx context.Context, repoDir string, terms []string) ([]KnowledgeMatch, error) {
	if len(terms) == 0 {
		return nil, nil
	}

	patternParts := make([]string, 0, len(terms))
	for _, term := range terms {
		patternParts = append(patternParts, regexp.QuoteMeta(term))
	}
	pattern := strings.Join(patternParts, "|")

	cmd := exec.CommandContext(
		ctx,
		"rg",
		"-n",
		"-i",
		"--no-heading",
		"--color", "never",
		"-S",
		"-e", pattern,
		"docs",
	)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// rg returns exit code 1 when no match is found.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("knowledge fulltext search failed: %s", trimOutput(out))
	}

	lines := strings.Split(string(out), "\n")
	matches := make([]KnowledgeMatch, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}

		lineNum, parseErr := strconv.Atoi(parts[1])
		if parseErr != nil {
			lineNum = 1
		}
		text := strings.TrimSpace(parts[2])
		if text == "" {
			continue
		}

		lower := strings.ToLower(text)
		termHits := 0
		for _, term := range terms {
			if strings.Contains(lower, term) {
				termHits++
			}
		}

		matches = append(matches, KnowledgeMatch{
			Path:    filepath.ToSlash(parts[0]),
			Line:    lineNum,
			Snippet: trimSnippet(text),
			Score:   20 + termHits*4,
			Reason:  "fulltext-rg",
		})
	}

	return matches, nil
}

func dedupeAndSortMatches(matches []KnowledgeMatch) []KnowledgeMatch {
	byKey := make(map[string]KnowledgeMatch, len(matches))
	for _, match := range matches {
		key := fmt.Sprintf("%s:%d", match.Path, match.Line)
		if existing, ok := byKey[key]; ok {
			if match.Score > existing.Score {
				byKey[key] = match
			}
			continue
		}
		byKey[key] = match
	}

	out := make([]KnowledgeMatch, 0, len(byKey))
	for _, match := range byKey {
		out = append(out, match)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})

	return out
}

func trimOutput(out []byte) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "unknown error"
	}
	if utf8.RuneCountInString(text) <= 300 {
		return text
	}
	runes := []rune(text)
	return string(runes[:300]) + "..."
}

func trimSnippet(v string) string {
	text := strings.TrimSpace(v)
	if text == "" {
		return text
	}
	if utf8.RuneCountInString(text) <= maxSnippetLen {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxSnippetLen]) + "..."
}
