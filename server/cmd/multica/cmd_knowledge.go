package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/knowledge"
)

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Knowledge repository tools",
}

var knowledgeSearchCmd = &cobra.Command{
	Use:   "search <issue-id>",
	Short: "Search workspace knowledge for an issue context",
	Args:  cobra.ExactArgs(1),
	RunE:  runKnowledgeSearch,
}

func init() {
	knowledgeCmd.AddCommand(knowledgeSearchCmd)
	knowledgeSearchCmd.Flags().String("output", "table", "Output format: table or json")
	knowledgeSearchCmd.Flags().Int("top-k", knowledge.DefaultLookupTopK, "Maximum number of matches to return")
}

func runKnowledgeSearch(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	if client.WorkspaceID == "" {
		return fmt.Errorf("workspace ID is required: pass --workspace-id or set MULTICA_WORKSPACE_ID")
	}

	issueID := strings.TrimSpace(args[0])
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var issue map[string]any
	if err := client.GetJSON(ctx, "/api/issues/"+issueID, &issue); err != nil {
		return fmt.Errorf("get issue: %w", err)
	}

	var comments []map[string]any
	if err := client.GetJSON(ctx, "/api/issues/"+issueID+"/comments", &comments); err != nil {
		return fmt.Errorf("list issue comments: %w", err)
	}

	var repoCfg struct {
		RepoURL       string `json:"repo_url"`
		DefaultBranch string `json:"default_branch"`
		Enabled       bool   `json:"enabled"`
	}
	if err := client.GetJSON(ctx, "/api/workspaces/"+client.WorkspaceID+"/knowledge-repo", &repoCfg); err != nil {
		return fmt.Errorf("get workspace knowledge repo: %w", err)
	}
	if !repoCfg.Enabled {
		return fmt.Errorf("knowledge repository automation is disabled for this workspace")
	}
	if strings.TrimSpace(repoCfg.RepoURL) == "" {
		return fmt.Errorf("knowledge repository URL is not configured")
	}

	queryTexts := []string{
		strVal(issue, "title"),
		strVal(issue, "description"),
	}
	if len(comments) > 0 {
		lastComment := comments[len(comments)-1]
		queryTexts = append(queryTexts, strVal(lastComment, "content"))
	}

	topK, _ := cmd.Flags().GetInt("top-k")
	tmpDir, err := os.MkdirTemp("", "multica-knowledge-search-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	result, err := knowledge.LookupKnowledge(ctx, knowledge.LookupInput{
		WorkspaceID:   client.WorkspaceID,
		IssueID:       issueID,
		RepoURL:       repoCfg.RepoURL,
		DefaultBranch: repoCfg.DefaultBranch,
		QueryTexts:    queryTexts,
		RepoDir:       filepath.Join(tmpDir, "knowledge-repo"),
		TopK:          topK,
	})
	if err != nil {
		return fmt.Errorf("lookup knowledge: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}

	fmt.Fprintf(os.Stdout, "Issue: %s\n", issueID)
	fmt.Fprintf(os.Stdout, "Repo: %s\n", result.RepoURL)
	fmt.Fprintf(os.Stdout, "Branch: %s\n", result.DefaultBranch)
	fmt.Fprintf(os.Stdout, "Hit count: %d\n", result.HitCount)
	if result.NoMatchReason != "" {
		fmt.Fprintf(os.Stdout, "No match reason: %s\n", result.NoMatchReason)
	}
	fmt.Fprintln(os.Stdout)

	if len(result.QueryTerms) > 0 {
		fmt.Fprintln(os.Stdout, "Query terms:")
		for _, term := range result.QueryTerms {
			fmt.Fprintf(os.Stdout, "- %s\n", term)
		}
		fmt.Fprintln(os.Stdout)
	}

	headers := []string{"PATH", "LINE", "SCORE", "REASON", "SNIPPET"}
	rows := make([][]string, 0, len(result.Matches))
	for _, match := range result.Matches {
		rows = append(rows, []string{
			match.Path,
			fmt.Sprintf("%d", match.Line),
			fmt.Sprintf("%d", match.Score),
			match.Reason,
			match.Snippet,
		})
	}
	cli.PrintTable(os.Stdout, headers, rows)
	return nil
}
