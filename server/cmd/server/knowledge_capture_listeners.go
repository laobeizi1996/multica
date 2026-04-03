package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func registerKnowledgeCaptureListeners(bus *events.Bus, queries *db.Queries) {
	ctx := context.Background()

	bus.Subscribe(protocol.EventTaskDispatch, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		taskID, _ := payload["task_id"].(string)
		if taskID == "" {
			return
		}
		_, _ = queries.MarkKnowledgeCaptureRunRunning(ctx, parseUUID(taskID))
	})

	bus.Subscribe(protocol.EventTaskFailed, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		taskID, _ := payload["task_id"].(string)
		if taskID == "" {
			return
		}
		run, err := queries.GetKnowledgeCaptureRunByTaskID(ctx, parseUUID(taskID))
		if err != nil {
			return
		}
		task, taskErr := queries.GetAgentTask(ctx, parseUUID(taskID))
		errText := "knowledge capture task failed"
		if taskErr == nil && task.Error.Valid {
			errText = task.Error.String
		}
		if _, err := queries.MarkKnowledgeCaptureRunFailed(ctx, db.MarkKnowledgeCaptureRunFailedParams{
			TaskID: run.TaskID,
			Error:  util.StrToText(errText),
		}); err != nil {
			slog.Warn("failed to mark knowledge capture run as failed", "task_id", taskID, "error", err)
		}
		notifyKnowledgeCaptureFailure(ctx, queries, bus, run, errText)
	})

	bus.Subscribe(protocol.EventTaskCompleted, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		taskID, _ := payload["task_id"].(string)
		if taskID == "" {
			return
		}

		// If this task is itself a knowledge capture task, finalize it and stop.
		if run, err := queries.GetKnowledgeCaptureRunByTaskID(ctx, parseUUID(taskID)); err == nil {
			finalizeKnowledgeCaptureRun(ctx, queries, run)
			return
		}

		task, err := queries.GetAgentTask(ctx, parseUUID(taskID))
		if err != nil {
			return
		}
		issue, err := queries.GetIssue(ctx, task.IssueID)
		if err != nil {
			return
		}
		triggerKnowledgeCapture(ctx, queries, bus, issue, "task_completed")
	})

	bus.Subscribe(protocol.EventIssueUpdated, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		statusChanged, _ := payload["status_changed"].(bool)
		if !statusChanged {
			return
		}

		issuePayload, ok := payload["issue"]
		if !ok {
			return
		}

		issueID := ""
		workspaceID := ""
		status := ""

		switch issue := issuePayload.(type) {
		case handler.IssueResponse:
			issueID = issue.ID
			workspaceID = issue.WorkspaceID
			status = issue.Status
		case map[string]any:
			issueID, _ = issue["id"].(string)
			workspaceID, _ = issue["workspace_id"].(string)
			status, _ = issue["status"].(string)
		}

		if issueID == "" || workspaceID == "" || status != "done" {
			return
		}

		issue, err := queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{
			ID:          parseUUID(issueID),
			WorkspaceID: parseUUID(workspaceID),
		})
		if err != nil {
			return
		}

		triggerKnowledgeCapture(ctx, queries, bus, issue, "issue_done")
	})
}

func triggerKnowledgeCapture(ctx context.Context, queries *db.Queries, bus *events.Bus, issue db.Issue, triggerSource string) {
	workspaceID := util.UUIDToString(issue.WorkspaceID)
	issueID := util.UUIDToString(issue.ID)

	repo, err := queries.GetWorkspaceKnowledgeRepo(ctx, issue.WorkspaceID)
	if err != nil {
		createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
			WorkspaceID:   issue.WorkspaceID,
			IssueID:       issue.ID,
			TriggerSource: triggerSource,
			Status:        "skipped",
			DedupeStatus:  "leader",
			SkipReason:    util.StrToText("knowledge repository is not configured"),
		})
		return
	}

	if !repo.Enabled {
		createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
			WorkspaceID:   issue.WorkspaceID,
			IssueID:       issue.ID,
			TriggerSource: triggerSource,
			Status:        "skipped",
			DedupeStatus:  "leader",
			SkipReason:    util.StrToText("knowledge repository automation is disabled"),
		})
		return
	}
	if strings.TrimSpace(repo.RepoUrl) == "" {
		createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
			WorkspaceID:   issue.WorkspaceID,
			IssueID:       issue.ID,
			TriggerSource: triggerSource,
			Status:        "skipped",
			DedupeStatus:  "leader",
			SkipReason:    util.StrToText("knowledge repository URL is not configured"),
		})
		return
	}
	if !repo.CuratorAgentID.Valid {
		createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
			WorkspaceID:   issue.WorkspaceID,
			IssueID:       issue.ID,
			TriggerSource: triggerSource,
			Status:        "skipped",
			DedupeStatus:  "leader",
			SkipReason:    util.StrToText("curator_agent_id is not configured"),
		})
		return
	}

	// If there is an active run already, create a deduplicated run record.
	activeRun, err := queries.GetActiveKnowledgeCaptureRunByIssue(ctx, db.GetActiveKnowledgeCaptureRunByIssueParams{
		WorkspaceID: issue.WorkspaceID,
		IssueID:     issue.ID,
	})
	if err == nil {
		createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
			WorkspaceID:     issue.WorkspaceID,
			IssueID:         issue.ID,
			TriggerSource:   triggerSource,
			Status:          "deduplicated",
			DedupeStatus:    "merged",
			MergedIntoRunID: activeRun.ID,
			SkipReason:      util.StrToText("merged into an active knowledge capture run"),
			FinishedAt:      pgtype.Timestamptz{Time: nowUTC(), Valid: true},
		})
		return
	}

	agent, err := queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{
		ID:          repo.CuratorAgentID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil || !agent.RuntimeID.Valid {
		run, _ := createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
			WorkspaceID:   issue.WorkspaceID,
			IssueID:       issue.ID,
			TriggerSource: triggerSource,
			Status:        "failed",
			DedupeStatus:  "leader",
			Error:         util.StrToText("curator agent is unavailable or missing runtime"),
			FinishedAt:    pgtype.Timestamptz{Time: nowUTC(), Valid: true},
		})
		notifyKnowledgeCaptureFailure(ctx, queries, bus, run, "curator agent is unavailable or missing runtime")
		return
	}

	task, err := queries.CreateAgentTask(ctx, db.CreateAgentTaskParams{
		AgentID:   agent.ID,
		RuntimeID: agent.RuntimeID,
		IssueID:   issue.ID,
		Priority:  knowledgePriority(issue.Priority),
	})
	if err != nil {
		run, _ := createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
			WorkspaceID:   issue.WorkspaceID,
			IssueID:       issue.ID,
			TriggerSource: triggerSource,
			Status:        "failed",
			DedupeStatus:  "leader",
			Error:         util.StrToText("failed to enqueue curator task"),
			FinishedAt:    pgtype.Timestamptz{Time: nowUTC(), Valid: true},
		})
		notifyKnowledgeCaptureFailure(ctx, queries, bus, run, "failed to enqueue curator task")
		return
	}

	_, err = createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
		WorkspaceID:   issue.WorkspaceID,
		IssueID:       issue.ID,
		TriggerSource: triggerSource,
		Status:        "pending",
		DedupeStatus:  "leader",
		TaskID:        task.ID,
	})
	if err == nil {
		slog.Info("knowledge capture enqueued",
			"workspace_id", workspaceID,
			"issue_id", issueID,
			"task_id", util.UUIDToString(task.ID),
			"trigger_source", triggerSource,
		)
		return
	}

	if !isUniqueViolationError(err) {
		// Keep the task queue clean if run creation failed.
		_, _ = queries.CancelAgentTask(ctx, task.ID)
		run, _ := createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
			WorkspaceID:   issue.WorkspaceID,
			IssueID:       issue.ID,
			TriggerSource: triggerSource,
			Status:        "failed",
			DedupeStatus:  "leader",
			Error:         util.StrToText("failed to persist capture run"),
			FinishedAt:    pgtype.Timestamptz{Time: nowUTC(), Valid: true},
		})
		notifyKnowledgeCaptureFailure(ctx, queries, bus, run, "failed to persist capture run")
		return
	}

	// Dedup race: another run became active. Cancel this extra task and record deduped run.
	_, _ = queries.CancelAgentTask(ctx, task.ID)
	activeRun, activeErr := queries.GetActiveKnowledgeCaptureRunByIssue(ctx, db.GetActiveKnowledgeCaptureRunByIssueParams{
		WorkspaceID: issue.WorkspaceID,
		IssueID:     issue.ID,
	})
	if activeErr == nil {
		createKnowledgeCaptureRun(ctx, queries, db.CreateKnowledgeCaptureRunParams{
			WorkspaceID:     issue.WorkspaceID,
			IssueID:         issue.ID,
			TriggerSource:   triggerSource,
			Status:          "deduplicated",
			DedupeStatus:    "merged",
			MergedIntoRunID: activeRun.ID,
			SkipReason:      util.StrToText("merged into an active knowledge capture run"),
			FinishedAt:      pgtype.Timestamptz{Time: nowUTC(), Valid: true},
		})
	}
}

func finalizeKnowledgeCaptureRun(ctx context.Context, queries *db.Queries, run db.KnowledgeCaptureRun) {
	task, err := queries.GetAgentTask(ctx, run.TaskID)
	if err != nil {
		_, _ = queries.MarkKnowledgeCaptureRunFailed(ctx, db.MarkKnowledgeCaptureRunFailedParams{
			TaskID: run.TaskID,
			Error:  util.StrToText("task result not found"),
		})
		return
	}

	var result protocol.TaskCompletedPayload
	if len(task.Result) > 0 {
		_ = json.Unmarshal(task.Result, &result)
	}

	if result.PRURL != "" {
		_, _ = queries.MarkKnowledgeCaptureRunCompleted(ctx, db.MarkKnowledgeCaptureRunCompletedParams{
			TaskID: run.TaskID,
			PrUrl:  util.StrToText(result.PRURL),
		})
		return
	}

	if reason := extractKnowledgeSkipReason(result.Output); reason != "" {
		_, _ = queries.MarkKnowledgeCaptureRunSkipped(ctx, db.MarkKnowledgeCaptureRunSkippedParams{
			TaskID:     run.TaskID,
			SkipReason: util.StrToText(reason),
		})
		return
	}

	_, _ = queries.MarkKnowledgeCaptureRunCompleted(ctx, db.MarkKnowledgeCaptureRunCompletedParams{
		TaskID: run.TaskID,
		PrUrl:  pgtype.Text{},
	})
}

func extractKnowledgeSkipReason(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "curator reported no knowledge updates"
	}

	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "SKIP_KNOWLEDGE_CAPTURE:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "SKIP_KNOWLEDGE_CAPTURE:"))
		}
	}

	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "no reusable knowledge") || strings.Contains(lower, "nothing to capture") {
		if len(trimmed) > 240 {
			return trimmed[:240]
		}
		return trimmed
	}
	return ""
}

func createKnowledgeCaptureRun(ctx context.Context, queries *db.Queries, params db.CreateKnowledgeCaptureRunParams) (db.KnowledgeCaptureRun, error) {
	if params.DedupeStatus == "" {
		params.DedupeStatus = "leader"
	}
	run, err := queries.CreateKnowledgeCaptureRun(ctx, params)
	if err != nil {
		return db.KnowledgeCaptureRun{}, err
	}
	return run, nil
}

func notifyKnowledgeCaptureFailure(ctx context.Context, queries *db.Queries, bus *events.Bus, run db.KnowledgeCaptureRun, reason string) {
	issue, err := queries.GetIssue(ctx, run.IssueID)
	if err != nil {
		return
	}

	members, err := queries.ListMembers(ctx, run.WorkspaceID)
	if err != nil {
		return
	}

	for _, member := range members {
		if member.Role != "owner" && member.Role != "admin" {
			continue
		}
		item, err := queries.CreateInboxItem(ctx, db.CreateInboxItemParams{
			WorkspaceID:   run.WorkspaceID,
			RecipientType: "member",
			RecipientID:   member.UserID,
			Type:          "knowledge_capture_failed",
			Severity:      "attention",
			IssueID:       run.IssueID,
			Title:         fmt.Sprintf("Knowledge capture failed: %s", issue.Title),
			Body:          util.StrToText(reason),
			ActorType:     util.StrToText("system"),
			Details:       []byte(`{"source":"knowledge_capture"}`),
		})
		if err != nil {
			continue
		}
		resp := inboxItemToResponse(item)
		resp["issue_status"] = issue.Status
		if bus != nil {
			bus.Publish(events.Event{
				Type:        protocol.EventInboxNew,
				WorkspaceID: util.UUIDToString(run.WorkspaceID),
				ActorType:   "system",
				ActorID:     "",
				Payload:     map[string]any{"item": resp},
			})
		}
	}
}

func knowledgePriority(priority string) int32 {
	switch priority {
	case "urgent":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func isUniqueViolationError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
