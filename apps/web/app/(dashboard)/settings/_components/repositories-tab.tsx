"use client";

import { useEffect, useState } from "react";
import { Save, Plus, Trash2, FolderGit2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { toast } from "sonner";
import { useAuthStore } from "@/features/auth";
import { useWorkspaceStore } from "@/features/workspace";
import { api } from "@/shared/api";
import type { WorkspaceRepo } from "@/shared/types";

const VISIBILITIES = ["private", "public", "internal"] as const;

export function RepositoriesTab() {
  const user = useAuthStore((s) => s.user);
  const workspace = useWorkspaceStore((s) => s.workspace);
  const members = useWorkspaceStore((s) => s.members);
  const updateWorkspace = useWorkspaceStore((s) => s.updateWorkspace);

  const [repos, setRepos] = useState<WorkspaceRepo[]>(workspace?.repos ?? []);
  const [saving, setSaving] = useState(false);
  const [creatingRepo, setCreatingRepo] = useState(false);
  const [ghOwner, setGhOwner] = useState("");
  const [ghRepoName, setGhRepoName] = useState(workspace?.slug ?? "");
  const [ghVisibility, setGhVisibility] = useState<(typeof VISIBILITIES)[number]>("private");
  const [ghDescription, setGhDescription] = useState("");

  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManageWorkspace = currentMember?.role === "owner" || currentMember?.role === "admin";

  useEffect(() => {
    setRepos(workspace?.repos ?? []);
    setGhRepoName(workspace?.slug ?? "");
    setGhDescription(workspace ? `Repository for ${workspace.name}` : "");
  }, [workspace]);

  const handleSave = async () => {
    if (!workspace) return;
    setSaving(true);
    try {
      const updated = await api.updateWorkspace(workspace.id, { repos });
      updateWorkspace(updated);
      toast.success("Repositories saved");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to save repositories");
    } finally {
      setSaving(false);
    }
  };

  const handleAddRepo = () => {
    setRepos([...repos, { url: "", description: "" }]);
  };

  const handleRemoveRepo = (index: number) => {
    setRepos(repos.filter((_, i) => i !== index));
  };

  const handleRepoChange = (index: number, field: keyof WorkspaceRepo, value: string) => {
    setRepos(repos.map((r, i) => (i === index ? { ...r, [field]: value } : r)));
  };

  const handleCreateViaGh = async () => {
    if (!workspace || !ghRepoName.trim()) return;
    setCreatingRepo(true);
    try {
      const result = await api.createWorkspaceRepoFromGitHub(workspace.id, {
        owner: ghOwner.trim() || undefined,
        repo_name: ghRepoName.trim(),
        visibility: ghVisibility,
        description: ghDescription.trim() || undefined,
        add_to_workspace_repos: true,
      });
      updateWorkspace(result.workspace);
      setRepos((result.workspace.repos ?? []) as WorkspaceRepo[]);
      toast.success(`Repository created: ${result.github_repo.name_with_owner}`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to create repository via gh");
    } finally {
      setCreatingRepo(false);
    }
  };

  if (!workspace) return null;

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <h2 className="text-sm font-semibold">Repositories</h2>

        <Card>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">
              GitHub repositories associated with this workspace. Agents use these to clone and work on code.
            </p>
            <p className="text-xs text-muted-foreground">
              For the dedicated workspace knowledge repository, use the <span className="font-medium">Knowledge Repo</span> tab.
            </p>

            {repos.map((repo, index) => (
              <div key={index} className="flex gap-2">
                <div className="flex-1 space-y-1.5">
                  <Input
                    type="url"
                    value={repo.url}
                    onChange={(e) => handleRepoChange(index, "url", e.target.value)}
                    disabled={!canManageWorkspace}
                    placeholder="https://github.com/org/repo"
                    className="text-sm"
                  />
                  <Input
                    type="text"
                    value={repo.description}
                    onChange={(e) => handleRepoChange(index, "description", e.target.value)}
                    disabled={!canManageWorkspace}
                    placeholder="Description (e.g. Go backend + Next.js frontend)"
                    className="text-sm"
                  />
                </div>
                {canManageWorkspace && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="mt-0.5 shrink-0 text-muted-foreground hover:text-destructive"
                    onClick={() => handleRemoveRepo(index)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                )}
              </div>
            ))}

            {canManageWorkspace && (
              <div className="flex items-center justify-between pt-1">
                <Button variant="outline" size="sm" onClick={handleAddRepo}>
                  <Plus className="h-3 w-3" />
                  Add repository
                </Button>
                <Button
                  size="sm"
                  onClick={handleSave}
                  disabled={saving}
                >
                  <Save className="h-3 w-3" />
                  {saving ? "Saving..." : "Save"}
                </Button>
              </div>
            )}

            {!canManageWorkspace && (
              <p className="text-xs text-muted-foreground">
                Only admins and owners can manage repositories.
              </p>
            )}
          </CardContent>
        </Card>
      </section>

      <section className="space-y-4">
        <div className="flex items-center gap-2">
          <FolderGit2 className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Create Repository via Local gh</h2>
        </div>

        <Card>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">
              Create any workspace repository through local `gh` and auto-bind it here.
            </p>

            <div className="grid gap-2 md:grid-cols-3">
              <Input
                value={ghOwner}
                onChange={(e) => setGhOwner(e.target.value)}
                disabled={!canManageWorkspace}
                placeholder="Owner (optional)"
              />
              <Input
                value={ghRepoName}
                onChange={(e) => setGhRepoName(e.target.value)}
                disabled={!canManageWorkspace}
                placeholder="Repository name"
              />
              <Select
                value={ghVisibility}
                onValueChange={(v) =>
                  setGhVisibility((v as (typeof VISIBILITIES)[number] | null) ?? "private")
                }
                disabled={!canManageWorkspace}
              >
                <SelectTrigger size="sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {VISIBILITIES.map((visibility) => (
                    <SelectItem key={visibility} value={visibility}>
                      {visibility}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <Input
              value={ghDescription}
              onChange={(e) => setGhDescription(e.target.value)}
              disabled={!canManageWorkspace}
              placeholder="Repository description"
            />

            <Button
              size="sm"
              onClick={handleCreateViaGh}
              disabled={!canManageWorkspace || creatingRepo || !ghRepoName.trim()}
            >
              <FolderGit2 className="h-3.5 w-3.5" />
              {creatingRepo ? "Creating..." : "Create with gh and add"}
            </Button>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
