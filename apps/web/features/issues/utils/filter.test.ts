import { describe, it, expect } from "vitest";
import type { Issue, IssueProject, Project } from "@/shared/types";
import { filterIssues, type IssueFilters } from "./filter";

const NO_FILTER: IssueFilters = {
  statusFilters: [],
  priorityFilters: [],
  assigneeFilters: [],
  includeNoAssignee: false,
  creatorFilters: [],
  projectFilters: [],
  projectLabelFilters: [],
  projects: [],
};

function makeIssue(overrides: Partial<Issue> = {}): Issue {
  return {
    id: "i-1",
    workspace_id: "ws-1",
    number: 1,
    identifier: "MUL-1",
    title: "Test",
    description: null,
    status: "todo",
    priority: "medium",
    assignee_type: null,
    assignee_id: null,
    creator_type: "member",
    creator_id: "u-1",
    parent_issue_id: null,
    position: 0,
    due_date: null,
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T00:00:00Z",
    ...overrides,
  };
}

function makeIssueProject(id: string): IssueProject {
  return {
    id,
    workspace_id: "ws-1",
    parent_id: null,
    name: id,
    slug: id,
    description: "",
    kind: "general",
    status: "active",
  };
}

const issues: Issue[] = [
  makeIssue({
    id: "1",
    status: "todo",
    priority: "high",
    assignee_type: "member",
    assignee_id: "u-1",
    creator_type: "member",
    creator_id: "u-1",
    projects: [makeIssueProject("p-1")],
    primary_project_id: "p-1",
  }),
  makeIssue({
    id: "2",
    status: "in_progress",
    priority: "medium",
    assignee_type: "agent",
    assignee_id: "a-1",
    creator_type: "agent",
    creator_id: "a-1",
    projects: [makeIssueProject("p-2")],
    primary_project_id: "p-2",
  }),
  makeIssue({
    id: "3",
    status: "done",
    priority: "low",
    assignee_type: null,
    assignee_id: null,
    creator_type: "member",
    creator_id: "u-2",
  }),
  makeIssue({
    id: "4",
    status: "todo",
    priority: "urgent",
    assignee_type: "member",
    assignee_id: "u-2",
    creator_type: "member",
    creator_id: "u-1",
    projects: [makeIssueProject("p-3")],
    primary_project_id: "p-3",
  }),
];

const projects: Project[] = [
  {
    id: "p-1",
    workspace_id: "ws-1",
    parent_id: null,
    name: "General",
    slug: "general",
    description: "",
    kind: "general",
    status: "active",
    labels: [
      { id: "l-frontend", workspace_id: "ws-1", name: "Frontend", color: "blue", created_at: "", updated_at: "" },
    ],
    created_at: "",
    updated_at: "",
  },
  {
    id: "p-2",
    workspace_id: "ws-1",
    parent_id: null,
    name: "Infra",
    slug: "infra",
    description: "",
    kind: "theme",
    status: "active",
    labels: [
      { id: "l-backend", workspace_id: "ws-1", name: "Backend", color: "green", created_at: "", updated_at: "" },
    ],
    created_at: "",
    updated_at: "",
  },
  {
    id: "p-3",
    workspace_id: "ws-1",
    parent_id: null,
    name: "Cross",
    slug: "cross",
    description: "",
    kind: "epic",
    status: "active",
    labels: [
      { id: "l-frontend", workspace_id: "ws-1", name: "Frontend", color: "blue", created_at: "", updated_at: "" },
    ],
    created_at: "",
    updated_at: "",
  },
];

describe("filterIssues", () => {
  it("returns all issues when no filters are active", () => {
    expect(filterIssues(issues, NO_FILTER)).toHaveLength(4);
  });

  // --- Status ---
  it("filters by status", () => {
    const result = filterIssues(issues, { ...NO_FILTER, statusFilters: ["todo"] });
    expect(result.map((i) => i.id)).toEqual(["1", "4"]);
  });

  // --- Priority ---
  it("filters by priority", () => {
    const result = filterIssues(issues, { ...NO_FILTER, priorityFilters: ["high", "urgent"] });
    expect(result.map((i) => i.id)).toEqual(["1", "4"]);
  });

  // --- Assignee ---
  it("filters by specific assignee", () => {
    const result = filterIssues(issues, {
      ...NO_FILTER,
      assigneeFilters: [{ type: "member", id: "u-1" }],
    });
    expect(result.map((i) => i.id)).toEqual(["1"]);
  });

  it("filters by 'No assignee' only", () => {
    const result = filterIssues(issues, { ...NO_FILTER, includeNoAssignee: true });
    expect(result.map((i) => i.id)).toEqual(["3"]);
  });

  it("filters by assignee + No assignee combined", () => {
    const result = filterIssues(issues, {
      ...NO_FILTER,
      assigneeFilters: [{ type: "agent", id: "a-1" }],
      includeNoAssignee: true,
    });
    expect(result.map((i) => i.id)).toEqual(["2", "3"]);
  });

  it("hides assigned issues when only 'No assignee' is selected", () => {
    const result = filterIssues(issues, { ...NO_FILTER, includeNoAssignee: true });
    expect(result.every((i) => !i.assignee_id)).toBe(true);
  });

  // --- Creator ---
  it("filters by creator", () => {
    const result = filterIssues(issues, {
      ...NO_FILTER,
      creatorFilters: [{ type: "agent", id: "a-1" }],
    });
    expect(result.map((i) => i.id)).toEqual(["2"]);
  });

  // --- Combinations ---
  it("applies status + assignee filters together", () => {
    const result = filterIssues(issues, {
      ...NO_FILTER,
      statusFilters: ["todo"],
      assigneeFilters: [{ type: "member", id: "u-1" }],
    });
    expect(result.map((i) => i.id)).toEqual(["1"]);
  });

  it("applies status + priority + creator filters together", () => {
    const result = filterIssues(issues, {
      ...NO_FILTER,
      statusFilters: ["todo"],
      priorityFilters: ["urgent"],
      creatorFilters: [{ type: "member", id: "u-1" }],
    });
    expect(result.map((i) => i.id)).toEqual(["4"]);
  });

  it("filters by project", () => {
    const result = filterIssues(issues, {
      ...NO_FILTER,
      projectFilters: ["p-2"],
    });
    expect(result.map((i) => i.id)).toEqual(["2"]);
  });

  it("filters by project label", () => {
    const result = filterIssues(issues, {
      ...NO_FILTER,
      projectLabelFilters: ["l-frontend"],
      projects,
    });
    expect(result.map((i) => i.id)).toEqual(["1", "4"]);
  });
});
