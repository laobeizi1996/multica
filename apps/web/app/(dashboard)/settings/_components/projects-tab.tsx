"use client";

import { useMemo, useRef, useState } from "react";
import { FolderTree, Plus, Tag, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuthStore } from "@/features/auth";
import { useWorkspaceStore } from "@/features/workspace";
import { api } from "@/shared/api";
import type { Project } from "@/shared/types";

const PROJECT_KINDS: Array<Project["kind"]> = ["portfolio", "epic", "theme", "general"];
const LABEL_COLORS = ["blue", "green", "amber", "red", "purple", "gray"];

function flattenProjectTree(projects: Project[]) {
  const childrenByParent = new Map<string | null, Project[]>();
  for (const project of projects) {
    const parentId = project.parent_id ?? null;
    const current = childrenByParent.get(parentId) ?? [];
    current.push(project);
    childrenByParent.set(parentId, current);
  }

  for (const nodes of childrenByParent.values()) {
    nodes.sort((a, b) => a.name.localeCompare(b.name));
  }

  const visited = new Set<string>();
  const ordered: Array<{ project: Project; depth: number }> = [];

  const walk = (parentId: string | null, depth: number) => {
    for (const project of childrenByParent.get(parentId) ?? []) {
      if (visited.has(project.id)) continue;
      visited.add(project.id);
      ordered.push({ project, depth });
      walk(project.id, depth + 1);
    }
  };

  walk(null, 0);

  for (const project of projects) {
    if (visited.has(project.id)) continue;
    ordered.push({ project, depth: 0 });
    walk(project.id, 1);
  }

  return ordered;
}

export function ProjectsTab() {
  const user = useAuthStore((s) => s.user);
  const workspace = useWorkspaceStore((s) => s.workspace);
  const members = useWorkspaceStore((s) => s.members);
  const projects = useWorkspaceStore((s) => s.projects ?? []);
  const projectLabels = useWorkspaceStore((s) => s.projectLabels ?? []);
  const refreshProjects = useWorkspaceStore((s) => s.refreshProjects);
  const refreshProjectLabels = useWorkspaceStore((s) => s.refreshProjectLabels);

  const [projectName, setProjectName] = useState("");
  const [parentId, setParentId] = useState<string>("none");
  const [kind, setKind] = useState<Project["kind"]>("theme");
  const [creatingProject, setCreatingProject] = useState(false);

  const [labelName, setLabelName] = useState("");
  const [labelColor, setLabelColor] = useState("blue");
  const [creatingLabel, setCreatingLabel] = useState(false);
  const [deletingProjectId, setDeletingProjectId] = useState<string | null>(null);
  const [deletingLabelId, setDeletingLabelId] = useState<string | null>(null);
  const projectNameInputRef = useRef<HTMLInputElement | null>(null);

  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManageWorkspace = currentMember?.role === "owner" || currentMember?.role === "admin";
  const flattenedProjects = useMemo(() => flattenProjectTree(projects), [projects]);

  const handleCreateProject = async () => {
    if (!workspace || !projectName.trim()) return;
    setCreatingProject(true);
    try {
      await api.createProject(workspace.id, {
        name: projectName.trim(),
        parent_id: parentId === "none" ? null : parentId,
        kind,
      });
      await refreshProjects();
      setProjectName("");
      toast.success(parentId === "none" ? "Project created" : "Subproject created");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to create project");
    } finally {
      setCreatingProject(false);
    }
  };

  const handleDeleteProject = async (project: Project) => {
    if (!workspace) return;
    if (!window.confirm(`Delete project "${project.name}"?`)) return;
    setDeletingProjectId(project.id);
    try {
      await api.deleteProject(workspace.id, project.id);
      await refreshProjects();
      toast.success("Project deleted");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to delete project");
    } finally {
      setDeletingProjectId(null);
    }
  };

  const handleCreateLabel = async () => {
    if (!workspace || !labelName.trim()) return;
    setCreatingLabel(true);
    try {
      await api.createProjectLabel(workspace.id, {
        name: labelName.trim(),
        color: labelColor,
      });
      await refreshProjectLabels();
      setLabelName("");
      toast.success("Project label created");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to create project label");
    } finally {
      setCreatingLabel(false);
    }
  };

  const handleDeleteLabel = async (labelId: string) => {
    if (!workspace) return;
    if (!window.confirm("Delete this project label?")) return;
    setDeletingLabelId(labelId);
    try {
      await api.deleteProjectLabel(workspace.id, labelId);
      await refreshProjectLabels();
      await refreshProjects();
      toast.success("Project label deleted");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to delete project label");
    } finally {
      setDeletingLabelId(null);
    }
  };

  if (!workspace) return null;

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <div className="flex items-center gap-2">
          <FolderTree className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Project Tree</h2>
        </div>

        <Card>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">
              Create top-level projects or choose a parent project to create subprojects.
            </p>

            <div className="grid gap-2 md:grid-cols-[1fr_180px_140px_auto]">
              <Input
                ref={projectNameInputRef}
                value={projectName}
                onChange={(e) => setProjectName(e.target.value)}
                disabled={!canManageWorkspace}
                placeholder="Project name"
              />
              <Select value={parentId} onValueChange={(v) => setParentId(v ?? "none")} disabled={!canManageWorkspace}>
                <SelectTrigger size="sm">
                  <SelectValue placeholder="Parent project" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">No parent (root)</SelectItem>
                  {flattenedProjects.map(({ project, depth }) => (
                    <SelectItem key={project.id} value={project.id}>
                      {`${depth > 0 ? `${"· ".repeat(depth)}` : ""}${project.name}`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={kind} onValueChange={(v) => setKind(v as Project["kind"])} disabled={!canManageWorkspace}>
                <SelectTrigger size="sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PROJECT_KINDS.map((item) => (
                    <SelectItem key={item} value={item}>
                      {item}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                size="sm"
                onClick={handleCreateProject}
                disabled={!canManageWorkspace || creatingProject || !projectName.trim()}
              >
                <Plus className="h-3.5 w-3.5" />
                {creatingProject ? "Creating..." : parentId === "none" ? "Add project" : "Add subproject"}
              </Button>
            </div>

            <div className="space-y-1.5 pt-1">
              {flattenedProjects.length === 0 && (
                <p className="text-xs text-muted-foreground">No projects yet.</p>
              )}
              {flattenedProjects.map(({ project, depth }) => (
                <div
                  key={project.id}
                  className="flex items-center justify-between rounded-md border px-2.5 py-1.5"
                >
                  <div className="min-w-0 flex items-center gap-2">
                    <span style={{ marginLeft: `${depth * 14}px` }} />
                    <FolderTree className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    <span className="truncate text-sm font-medium">{project.name}</span>
                    <Badge variant="outline">{project.kind}</Badge>
                    {project.status === "archived" && <Badge variant="secondary">archived</Badge>}
                    {project.labels.map((label) => (
                      <Badge key={label.id} variant="secondary">{label.name}</Badge>
                    ))}
                  </div>
                  {canManageWorkspace && (
                    <div className="flex items-center gap-1">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          setParentId(project.id);
                          projectNameInputRef.current?.focus();
                        }}
                      >
                        Add child
                      </Button>
                      {project.kind !== "general" && (
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => handleDeleteProject(project)}
                          disabled={deletingProjectId === project.id}
                          className="text-muted-foreground hover:text-destructive"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      )}
                    </div>
                  )}
                </div>
              ))}
            </div>

            {!canManageWorkspace && (
              <p className="text-xs text-muted-foreground">
                Only admins and owners can create, edit, or delete projects.
              </p>
            )}
          </CardContent>
        </Card>
      </section>

      <section className="space-y-4">
        <div className="flex items-center gap-2">
          <Tag className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Project Labels</h2>
        </div>

        <Card>
          <CardContent className="space-y-3">
            <div className="grid gap-2 md:grid-cols-[1fr_140px_auto]">
              <Input
                value={labelName}
                onChange={(e) => setLabelName(e.target.value)}
                disabled={!canManageWorkspace}
                placeholder="Label name"
              />
              <Select value={labelColor} onValueChange={(v) => setLabelColor(v ?? "blue")} disabled={!canManageWorkspace}>
                <SelectTrigger size="sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {LABEL_COLORS.map((color) => (
                    <SelectItem key={color} value={color}>{color}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                size="sm"
                onClick={handleCreateLabel}
                disabled={!canManageWorkspace || creatingLabel || !labelName.trim()}
              >
                <Plus className="h-3.5 w-3.5" />
                {creatingLabel ? "Creating..." : "Add label"}
              </Button>
            </div>

            <div className="space-y-1.5">
              {projectLabels.length === 0 && (
                <p className="text-xs text-muted-foreground">No labels yet.</p>
              )}
              {projectLabels.map((label) => (
                <div key={label.id} className="flex items-center justify-between rounded-md border px-2.5 py-1.5">
                  <div className="flex items-center gap-2">
                    <Badge variant="secondary">{label.name}</Badge>
                    <span className="text-xs text-muted-foreground">{label.color}</span>
                  </div>
                  {canManageWorkspace && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => handleDeleteLabel(label.id)}
                      disabled={deletingLabelId === label.id}
                      className="text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  )}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
