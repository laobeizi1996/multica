export interface WorkspaceKnowledgeRepo {
  workspace_id: string;
  repo_url: string;
  default_branch: string;
  curator_agent_id: string | null;
  template_version: string;
  mode: "pr";
  enabled: boolean;
  last_bootstrapped_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface KnowledgeTemplateEntry {
  path: string;
  type: "file" | "dir";
  content?: string;
}

export interface KnowledgeCaptureRun {
  id: string;
  workspace_id: string;
  issue_id: string;
  trigger_source: "task_completed" | "issue_done";
  status: "pending" | "running" | "completed" | "skipped" | "failed" | "deduplicated";
  dedupe_status: "leader" | "merged";
  merged_into_run_id: string | null;
  task_id: string | null;
  pr_url: string | null;
  skip_reason: string | null;
  error: string | null;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
}
