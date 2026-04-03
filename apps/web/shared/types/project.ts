export interface ProjectLabel {
  id: string;
  workspace_id: string;
  name: string;
  color: string;
  created_at: string;
  updated_at: string;
}

export interface Project {
  id: string;
  workspace_id: string;
  parent_id: string | null;
  name: string;
  slug: string;
  description: string;
  kind: "portfolio" | "epic" | "theme" | "general";
  status: "active" | "archived";
  labels: ProjectLabel[];
  created_at: string;
  updated_at: string;
}

export interface ProjectTreeNode {
  project: Project;
  children: ProjectTreeNode[];
}
