"use client";

import { useMemo, useState } from "react";
import { Check, Circle, CircleDot, FolderTree } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useWorkspaceStore } from "@/features/workspace";
import type { Project } from "@/shared/types";
import { cn } from "@/lib/utils";

interface ProjectSelection {
  project_ids: string[];
  primary_project_id: string | null;
}

function flattenProjectTree(projects: Project[]) {
  const childrenByParent = new Map<string | null, Project[]>();
  for (const project of projects) {
    const parentId = project.parent_id ?? null;
    const current = childrenByParent.get(parentId) ?? [];
    current.push(project);
    childrenByParent.set(parentId, current);
  }

  for (const list of childrenByParent.values()) {
    list.sort((a, b) => a.name.localeCompare(b.name));
  }

  const visited = new Set<string>();
  const flattened: Array<{ projectId: string; name: string; depth: number; archived: boolean }> = [];

  const walk = (parentId: string | null, depth: number) => {
    for (const project of childrenByParent.get(parentId) ?? []) {
      if (visited.has(project.id)) continue;
      visited.add(project.id);
      flattened.push({
        projectId: project.id,
        name: project.name,
        depth,
        archived: project.status === "archived",
      });
      walk(project.id, depth + 1);
    }
  };

  walk(null, 0);

  for (const project of projects) {
    if (visited.has(project.id)) continue;
    flattened.push({
      projectId: project.id,
      name: project.name,
      depth: 0,
      archived: project.status === "archived",
    });
    walk(project.id, 1);
  }

  return flattened;
}

export function ProjectPicker({
  projectIds,
  primaryProjectId,
  onChange,
  trigger: customTrigger,
  triggerRender,
  open: controlledOpen,
  onOpenChange: controlledOnOpenChange,
  align = "start",
  placeholder = "No project",
}: {
  projectIds: string[];
  primaryProjectId: string | null;
  onChange: (selection: ProjectSelection) => void;
  trigger?: React.ReactNode;
  triggerRender?: React.ReactElement;
  open?: boolean;
  onOpenChange?: (v: boolean) => void;
  align?: "start" | "center" | "end";
  placeholder?: string;
}) {
  const projects = useWorkspaceStore((s) => s.projects ?? []);
  const projectById = useMemo(
    () => new Map(projects.map((project) => [project.id, project])),
    [projects],
  );
  const [internalOpen, setInternalOpen] = useState(false);
  const open = controlledOpen ?? internalOpen;
  const setOpen = controlledOnOpenChange ?? setInternalOpen;
  const [filter, setFilter] = useState("");

  const flattened = useMemo(() => flattenProjectTree(projects), [projects]);
  const query = filter.toLowerCase();
  const visibleProjects = flattened.filter((project) =>
    !query || project.name.toLowerCase().includes(query),
  );

  const selectedProjects = projectIds
    .map((id) => projectById.get(id))
    .filter(Boolean);
  const primaryName = primaryProjectId
    ? (projectById.get(primaryProjectId)?.name ?? null)
    : null;

  const handleToggle = (projectId: string) => {
    const selected = new Set(projectIds);
    if (selected.has(projectId)) {
      selected.delete(projectId);
    } else {
      selected.add(projectId);
    }

    const nextProjectIds = Array.from(selected);
    const nextPrimary = nextProjectIds.includes(primaryProjectId ?? "")
      ? primaryProjectId
      : (nextProjectIds[0] ?? null);
    onChange({
      project_ids: nextProjectIds,
      primary_project_id: nextPrimary,
    });
  };

  const handleSetPrimary = (projectId: string) => {
    const selected = new Set(projectIds);
    selected.add(projectId);
    onChange({
      project_ids: Array.from(selected),
      primary_project_id: projectId,
    });
  };

  return (
    <Popover
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        if (!v) setFilter("");
      }}
    >
      <PopoverTrigger
        className={triggerRender ? undefined : "flex items-center gap-1.5 cursor-pointer rounded px-1 -mx-1 hover:bg-accent/30 transition-colors overflow-hidden"}
        render={triggerRender}
      >
        {customTrigger ?? (
          <>
            <FolderTree className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            {selectedProjects.length === 0 ? (
              <span className="truncate text-muted-foreground">{placeholder}</span>
            ) : (
              <span className="truncate">
                {primaryName ?? selectedProjects[0]?.name}
                {selectedProjects.length > 1
                  ? ` +${selectedProjects.length - 1}`
                  : ""}
              </span>
            )}
          </>
        )}
      </PopoverTrigger>

      <PopoverContent align={align} className="w-72 gap-0 p-0">
        <div className="px-2 py-1.5 border-b">
          <input
            type="text"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter projects..."
            className="w-full bg-transparent text-sm placeholder:text-muted-foreground outline-none"
          />
        </div>
        <div className="p-1 max-h-72 overflow-y-auto">
          {visibleProjects.map((project) => {
            const selected = projectIds.includes(project.projectId);
            const primary = primaryProjectId === project.projectId;
            return (
              <div
                key={project.projectId}
                role="button"
                tabIndex={0}
                onClick={() => handleToggle(project.projectId)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    handleToggle(project.projectId);
                  }
                }}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-accent transition-colors cursor-pointer"
              >
                <span className={cn("flex items-center gap-2 flex-1 min-w-0", project.archived && "text-muted-foreground")}>
                  <span
                    className="inline-block w-0 shrink-0"
                    style={{ marginLeft: `${project.depth * 10}px` }}
                  />
                  <FolderTree className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate">{project.name}</span>
                  {project.archived && (
                    <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                      archived
                    </span>
                  )}
                </span>
                {selected && (
                  <button
                    type="button"
                    onClick={(event) => {
                      event.stopPropagation();
                      handleSetPrimary(project.projectId);
                    }}
                    className={cn(
                      "inline-flex items-center gap-1 rounded px-1 py-0.5 text-[10px] border",
                      primary
                        ? "border-primary text-primary"
                        : "border-border text-muted-foreground hover:text-foreground",
                    )}
                    title="Set as primary project"
                  >
                    {primary ? <CircleDot className="h-3 w-3" /> : <Circle className="h-3 w-3" />}
                    Primary
                  </button>
                )}
                {selected && <Check className="h-3.5 w-3.5 text-muted-foreground" />}
              </div>
            );
          })}

          {visibleProjects.length === 0 && (
            <div className="px-2 py-3 text-center text-sm text-muted-foreground">
              No results
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
