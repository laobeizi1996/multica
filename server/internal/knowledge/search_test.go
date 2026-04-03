package knowledge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtractQueryTerms(t *testing.T) {
	terms := extractQueryTerms([]string{
		"Fix login redirect bug in workspace settings",
		"Please update login flow and settings page behavior",
	})
	if len(terms) == 0 {
		t.Fatal("expected extracted terms, got none")
	}
	if terms[0] == "the" || terms[0] == "and" {
		t.Fatalf("unexpected stopword in extracted terms: %q", terms[0])
	}
}

func TestLookupKnowledgeFindsPriorityAndFulltextMatches(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoRoot := t.TempDir()
	writeFile(t, repoRoot, "AGENTS.md", "# Agents\nUse login best practices.")
	writeFile(t, repoRoot, "ARCHITECTURE.md", "# Architecture\nAuth service and session control.")
	writeFile(t, repoRoot, "docs/README.md", "# Docs\n")
	writeFile(t, repoRoot, "docs/design-docs/index.md", "# Index\n")
	writeFile(t, repoRoot, "docs/design-docs/design-doc1.md", "Handle login redirect when auth token expires.")

	run(t, repoRoot, "git", "init")
	run(t, repoRoot, "git", "config", "user.email", "test@example.com")
	run(t, repoRoot, "git", "config", "user.name", "test")
	run(t, repoRoot, "git", "add", ".")
	run(t, repoRoot, "git", "commit", "-m", "init")

	bareRepo := filepath.Join(t.TempDir(), "knowledge.git")
	run(t, "", "git", "clone", "--bare", repoRoot, bareRepo)

	lookupDir := filepath.Join(t.TempDir(), "lookup")
	result, err := LookupKnowledge(context.Background(), LookupInput{
		WorkspaceID: "ws-1",
		IssueID:     "iss-1",
		RepoURL:     bareRepo,
		QueryTexts:  []string{"login redirect fails after auth expires"},
		RepoDir:     lookupDir,
		TopK:        8,
	})
	if err != nil {
		t.Fatalf("LookupKnowledge returned error: %v", err)
	}
	if result.HitCount == 0 {
		t.Fatalf("expected matches, got none: %+v", result)
	}
	if result.Matches[0].Path == "" {
		t.Fatal("expected non-empty match path")
	}
}

func TestLookupKnowledgeNoTermsDoesNotFail(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoRoot := t.TempDir()
	writeFile(t, repoRoot, "AGENTS.md", "# Agents\n")
	writeFile(t, repoRoot, "ARCHITECTURE.md", "# Architecture\n")
	writeFile(t, repoRoot, "docs/README.md", "# Docs\n")
	run(t, repoRoot, "git", "init")
	run(t, repoRoot, "git", "config", "user.email", "test@example.com")
	run(t, repoRoot, "git", "config", "user.name", "test")
	run(t, repoRoot, "git", "add", ".")
	run(t, repoRoot, "git", "commit", "-m", "init")

	bareRepo := filepath.Join(t.TempDir(), "knowledge.git")
	run(t, "", "git", "clone", "--bare", repoRoot, bareRepo)

	result, err := LookupKnowledge(context.Background(), LookupInput{
		WorkspaceID: "ws-1",
		IssueID:     "iss-1",
		RepoURL:     bareRepo,
		QueryTexts:  []string{"a an the and"},
		RepoDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("LookupKnowledge returned error: %v", err)
	}
	if result.NoMatchReason == "" {
		t.Fatal("expected no_match_reason when no query terms are extracted")
	}
}

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", relPath, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v: %s", name, args, err, string(out))
	}
}
