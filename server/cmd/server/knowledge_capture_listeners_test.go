package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func firstWorkspaceAgentID(t *testing.T) string {
	t.Helper()
	var agentID string
	err := testPool.QueryRow(context.Background(), `
		SELECT id FROM agent
		WHERE workspace_id = $1
		ORDER BY created_at ASC
		LIMIT 1
	`, testWorkspaceID).Scan(&agentID)
	if err != nil {
		t.Fatalf("failed to load test workspace agent: %v", err)
	}
	return agentID
}

func setupKnowledgeRepoForTests(t *testing.T, queries *db.Queries) {
	t.Helper()
	agentID := firstWorkspaceAgentID(t)
	_, err := queries.UpsertWorkspaceKnowledgeRepo(context.Background(), db.UpsertWorkspaceKnowledgeRepoParams{
		WorkspaceID:     util.ParseUUID(testWorkspaceID),
		RepoUrl:         "https://github.com/example/knowledge-repo",
		DefaultBranch:   "main",
		CuratorAgentID:  util.ParseUUID(agentID),
		TemplateVersion: "openai-harness-v1",
		Mode:            "pr",
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("failed to configure workspace knowledge repo: %v", err)
	}
}

func TestKnowledgeCaptureTriggerDeduplicatesWithinActiveWindow(t *testing.T) {
	queries := db.New(testPool)
	setupKnowledgeRepoForTests(t, queries)

	issueID := createTestIssue(t, testWorkspaceID, testUserID)
	t.Cleanup(func() { cleanupTestIssue(t, issueID) })

	issue, err := queries.GetIssue(context.Background(), util.ParseUUID(issueID))
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	triggerKnowledgeCapture(context.Background(), queries, nil, issue, "issue_done")
	triggerKnowledgeCapture(context.Background(), queries, nil, issue, "task_completed")

	var pendingCount int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*)::int
		FROM knowledge_capture_run
		WHERE issue_id = $1 AND status = 'pending'
	`, issueID).Scan(&pendingCount); err != nil {
		t.Fatalf("count pending capture runs: %v", err)
	}
	if pendingCount != 1 {
		t.Fatalf("expected 1 pending run, got %d", pendingCount)
	}

	var dedupCount int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*)::int
		FROM knowledge_capture_run
		WHERE issue_id = $1 AND status = 'deduplicated'
	`, issueID).Scan(&dedupCount); err != nil {
		t.Fatalf("count deduplicated capture runs: %v", err)
	}
	if dedupCount != 1 {
		t.Fatalf("expected 1 deduplicated run, got %d", dedupCount)
	}

	var activeTasks int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*)::int
		FROM agent_task_queue
		WHERE issue_id = $1 AND status IN ('queued', 'dispatched', 'running')
	`, issueID).Scan(&activeTasks); err != nil {
		t.Fatalf("count active tasks: %v", err)
	}
	if activeTasks != 1 {
		t.Fatalf("expected exactly 1 active curator task, got %d", activeTasks)
	}
}

func TestFinalizeKnowledgeCaptureRunMarksSkippedFromOutput(t *testing.T) {
	queries := db.New(testPool)
	setupKnowledgeRepoForTests(t, queries)

	issueID := createTestIssue(t, testWorkspaceID, testUserID)
	t.Cleanup(func() { cleanupTestIssue(t, issueID) })

	issue, err := queries.GetIssue(context.Background(), util.ParseUUID(issueID))
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	triggerKnowledgeCapture(context.Background(), queries, nil, issue, "issue_done")

	var taskID string
	if err := testPool.QueryRow(context.Background(), `
		SELECT task_id::text
		FROM knowledge_capture_run
		WHERE issue_id = $1 AND status = 'pending'
		ORDER BY created_at DESC
		LIMIT 1
	`, issueID).Scan(&taskID); err != nil {
		t.Fatalf("failed to load pending capture task id: %v", err)
	}

	resultBytes, _ := json.Marshal(protocol.TaskCompletedPayload{
		TaskID: taskID,
		Output: "SKIP_KNOWLEDGE_CAPTURE: no reusable knowledge in this issue",
	})
	if _, err := testPool.Exec(context.Background(), `
		UPDATE agent_task_queue
		SET status = 'completed', result = $2::jsonb, completed_at = now()
		WHERE id = $1
	`, taskID, resultBytes); err != nil {
		t.Fatalf("failed to update task result: %v", err)
	}

	run, err := queries.GetKnowledgeCaptureRunByTaskID(context.Background(), util.ParseUUID(taskID))
	if err != nil {
		t.Fatalf("GetKnowledgeCaptureRunByTaskID: %v", err)
	}
	finalizeKnowledgeCaptureRun(context.Background(), queries, run)

	updated, err := queries.GetKnowledgeCaptureRunByTaskID(context.Background(), util.ParseUUID(taskID))
	if err != nil {
		t.Fatalf("GetKnowledgeCaptureRunByTaskID(updated): %v", err)
	}
	if updated.Status != "skipped" {
		t.Fatalf("expected run status skipped, got %s", updated.Status)
	}
	if !updated.SkipReason.Valid || updated.SkipReason.String == "" {
		t.Fatal("expected skip_reason to be populated")
	}
}
