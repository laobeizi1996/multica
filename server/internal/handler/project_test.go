package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func withURLParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestProjectAndIssueAssociationFlow(t *testing.T) {
	// Create project label
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/project-labels", map[string]any{
		"name":  "Epic Theme",
		"color": "blue",
	})
	req = withURLParams(req, map[string]string{"id": testWorkspaceID})
	testHandler.CreateProjectLabel(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProjectLabel: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var label ProjectLabelResponse
	_ = json.NewDecoder(w.Body).Decode(&label)

	// Create project with label
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/projects", map[string]any{
		"name":      "Knowledge Revamp",
		"kind":      "epic",
		"status":    "active",
		"label_ids": []string{label.ID},
	})
	req = withURLParams(req, map[string]string{"id": testWorkspaceID})
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	_ = json.NewDecoder(w.Body).Decode(&project)
	if len(project.Labels) != 1 || project.Labels[0].ID != label.ID {
		t.Fatalf("CreateProject: expected project label %s to be attached", label.ID)
	}

	// Create issue with explicit project binding
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":              "Project-bound issue",
		"status":             "todo",
		"project_ids":        []string{project.ID},
		"primary_project_id": project.ID,
	})
	testHandler.CreateIssue(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateIssue: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var issue IssueResponse
	_ = json.NewDecoder(w.Body).Decode(&issue)
	if issue.PrimaryProjectID == nil || *issue.PrimaryProjectID != project.ID {
		t.Fatalf("CreateIssue: expected primary_project_id=%s, got %#v", project.ID, issue.PrimaryProjectID)
	}
	if len(issue.Projects) != 1 || issue.Projects[0].ID != project.ID {
		t.Fatalf("CreateIssue: expected issue to include project %s", project.ID)
	}

	// List issues by project filter
	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/issues?workspace_id="+testWorkspaceID+"&project_id="+project.ID, nil)
	testHandler.ListIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListIssues: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var listResp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&listResp)
	issues := listResp["issues"].([]any)
	if len(issues) == 0 {
		t.Fatal("ListIssues: expected filtered result to include created issue")
	}

	// Update project
	w = httptest.NewRecorder()
	req = newRequest("PATCH", "/api/workspaces/"+testWorkspaceID+"/projects/"+project.ID, map[string]any{
		"status": "archived",
	})
	req = withURLParams(req, map[string]string{"id": testWorkspaceID, "projectId": project.ID})
	testHandler.UpdateProject(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateProject: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated ProjectResponse
	_ = json.NewDecoder(w.Body).Decode(&updated)
	if updated.Status != "archived" {
		t.Fatalf("UpdateProject: expected archived, got %s", updated.Status)
	}

	// Cleanup issue
	w = httptest.NewRecorder()
	req = newRequest("DELETE", "/api/issues/"+issue.ID, nil)
	req = withURLParam(req, "id", issue.ID)
	testHandler.DeleteIssue(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DeleteIssue: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Cleanup project
	w = httptest.NewRecorder()
	req = newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/projects/"+project.ID, nil)
	req = withURLParams(req, map[string]string{"id": testWorkspaceID, "projectId": project.ID})
	testHandler.DeleteProject(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DeleteProject: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Cleanup label
	w = httptest.NewRecorder()
	req = newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/project-labels/"+label.ID, nil)
	req = withURLParams(req, map[string]string{"id": testWorkspaceID, "labelId": label.ID})
	testHandler.DeleteProjectLabel(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DeleteProjectLabel: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}
