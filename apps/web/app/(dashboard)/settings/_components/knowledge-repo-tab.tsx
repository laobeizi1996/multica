"use client";

import { useEffect, useMemo, useState } from "react";
import { BookCopy, FolderGit2, Save, Sparkles, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
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
import type { KnowledgeTemplateEntry, WorkspaceKnowledgeRepo } from "@/shared/types";

const VISIBILITIES = ["private", "public", "internal"] as const;

export function KnowledgeRepoTab() {
  const user = useAuthStore((s) => s.user);
  const workspace = useWorkspaceStore((s) => s.workspace);
  const members = useWorkspaceStore((s) => s.members);
  const agents = useWorkspaceStore((s) => s.agents);
  const updateWorkspace = useWorkspaceStore((s) => s.updateWorkspace);

  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [bootstrapping, setBootstrapping] = useState(false);
  const [validating, setValidating] = useState(false);
  const [creatingGithubRepo, setCreatingGithubRepo] = useState(false);

  const [repoConfig, setRepoConfig] = useState<WorkspaceKnowledgeRepo | null>(null);
  const [templateEntries, setTemplateEntries] = useState<KnowledgeTemplateEntry[]>([]);

  const [repoUrl, setRepoUrl] = useState("");
  const [defaultBranch, setDefaultBranch] = useState("main");
  const [curatorAgentId, setCuratorAgentId] = useState<string>("none");
  const [enabled, setEnabled] = useState(true);

  const [ghOwner, setGhOwner] = useState("");
  const [ghRepoName, setGhRepoName] = useState("");
  const [ghVisibility, setGhVisibility] = useState<(typeof VISIBILITIES)[number]>("private");

  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManageWorkspace = currentMember?.role === "owner" || currentMember?.role === "admin";

  const detectedRepoName = useMemo(() => {
    if (!workspace) return "";
    return workspace.slug ? `${workspace.slug}-knowledge` : "knowledge-base";
  }, [workspace]);

  useEffect(() => {
    if (!workspace) return;
    setGhRepoName(detectedRepoName);
  }, [workspace, detectedRepoName]);

  useEffect(() => {
    const load = async () => {
      if (!workspace) return;
      setLoading(true);
      try {
        const config = await api.getWorkspaceKnowledgeRepo(workspace.id);
        setRepoConfig(config);
        setRepoUrl(config.repo_url ?? "");
        setDefaultBranch(config.default_branch || "main");
        setCuratorAgentId(config.curator_agent_id ?? "none");
        setEnabled(config.enabled);
      } catch (e) {
        toast.error(e instanceof Error ? e.message : "Failed to load knowledge repo config");
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [workspace]);

  const handleSave = async () => {
    if (!workspace) return;
    setSaving(true);
    try {
      const config = await api.updateWorkspaceKnowledgeRepo(workspace.id, {
        repo_url: repoUrl.trim(),
        default_branch: defaultBranch.trim() || "main",
        curator_agent_id: curatorAgentId === "none" ? null : curatorAgentId,
        enabled,
      });
      setRepoConfig(config);
      const updatedWorkspace = await api.getWorkspace(workspace.id);
      updateWorkspace(updatedWorkspace);
      toast.success("Knowledge repo configuration saved");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to save knowledge repo configuration");
    } finally {
      setSaving(false);
    }
  };

  const handleBootstrap = async () => {
    if (!workspace) return;
    setBootstrapping(true);
    try {
      const result = await api.bootstrapWorkspaceKnowledgeRepo(workspace.id);
      setRepoConfig(result.knowledge_repo);
      setTemplateEntries(result.entries);
      toast.success("Knowledge repo template bootstrapped");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to bootstrap knowledge repo");
    } finally {
      setBootstrapping(false);
    }
  };

  const handleValidate = async () => {
    if (!workspace) return;
    if (templateEntries.length === 0) {
      toast.info("Bootstrap once to load template entries before validation");
      return;
    }
    setValidating(true);
    try {
      const result = await api.validateWorkspaceKnowledgeRepo(workspace.id, templateEntries);
      if (result.valid) {
        toast.success("Knowledge template validation passed");
        return;
      }
      const topError = result.errors[0] ?? "Validation failed";
      toast.error(topError);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to validate knowledge repo");
    } finally {
      setValidating(false);
    }
  };

  const handleCreateGithubRepo = async () => {
    if (!workspace || !ghRepoName.trim()) return;
    setCreatingGithubRepo(true);
    try {
      const result = await api.createWorkspaceKnowledgeRepoFromGitHub(workspace.id, {
        owner: ghOwner.trim() || undefined,
        repo_name: ghRepoName.trim(),
        visibility: ghVisibility,
        default_branch: defaultBranch.trim() || "main",
        add_to_workspace_repos: true,
      });
      setRepoConfig(result.knowledge_repo);
      setRepoUrl(result.github_repo.url);
      setDefaultBranch(result.github_repo.default_branch);
      const updatedWorkspace = await api.getWorkspace(workspace.id);
      updateWorkspace(updatedWorkspace);
      toast.success(`GitHub repo created: ${result.github_repo.name_with_owner}`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to create GitHub repository with gh");
    } finally {
      setCreatingGithubRepo(false);
    }
  };

  if (!workspace) return null;

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <div className="flex items-center gap-2">
          <BookCopy className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Knowledge Repository</h2>
        </div>

        <Card>
          <CardContent className="space-y-4">
            <p className="text-xs text-muted-foreground">
              Configure an independent knowledge repository for this workspace. Curator agents use this config to open PRs with reusable knowledge.
            </p>

            <div className="space-y-2">
              <Label className="text-xs text-muted-foreground">Repository URL</Label>
              <Input
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                disabled={!canManageWorkspace || loading}
                placeholder="https://github.com/org/repo"
              />
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-2">
                <Label className="text-xs text-muted-foreground">Default Branch</Label>
                <Input
                  value={defaultBranch}
                  onChange={(e) => setDefaultBranch(e.target.value)}
                  disabled={!canManageWorkspace || loading}
                  placeholder="main"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-xs text-muted-foreground">Curator Agent</Label>
                <Select
                  value={curatorAgentId}
                  onValueChange={(v) => setCuratorAgentId(v ?? "none")}
                  disabled={!canManageWorkspace || loading}
                >
                  <SelectTrigger size="sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">No curator agent</SelectItem>
                    {agents.map((agent) => (
                      <SelectItem key={agent.id} value={agent.id}>
                        {agent.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="flex items-center justify-between rounded-md border px-3 py-2">
              <div>
                <p className="text-sm font-medium">Enable Automatic Knowledge Capture</p>
                <p className="text-xs text-muted-foreground">
                  Trigger curator tasks on issue/task completion.
                </p>
              </div>
              <Switch
                checked={enabled}
                onCheckedChange={setEnabled}
                disabled={!canManageWorkspace || loading}
              />
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                onClick={handleSave}
                disabled={!canManageWorkspace || saving || loading}
              >
                <Save className="h-3.5 w-3.5" />
                {saving ? "Saving..." : "Save config"}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={handleBootstrap}
                disabled={!canManageWorkspace || bootstrapping || loading}
              >
                <Sparkles className="h-3.5 w-3.5" />
                {bootstrapping ? "Bootstrapping..." : "Bootstrap template"}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={handleValidate}
                disabled={validating || loading}
              >
                <ShieldCheck className="h-3.5 w-3.5" />
                {validating ? "Validating..." : "Validate"}
              </Button>
            </div>

            {repoConfig?.last_bootstrapped_at && (
              <p className="text-xs text-muted-foreground">
                Last bootstrapped: {new Date(repoConfig.last_bootstrapped_at).toLocaleString()}
              </p>
            )}

            {!canManageWorkspace && (
              <p className="text-xs text-muted-foreground">
                Only admins and owners can update knowledge repository settings.
              </p>
            )}
          </CardContent>
        </Card>
      </section>

      <section className="space-y-4">
        <div className="flex items-center gap-2">
          <FolderGit2 className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Create GitHub Repo via Local gh</h2>
        </div>

        <Card>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">
              Uses server-side `gh` CLI on this machine. Ensure `gh auth status` is already logged in.
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
                onValueChange={(v) => setGhVisibility((v as (typeof VISIBILITIES)[number] | null) ?? "private")}
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
            <Button
              size="sm"
              onClick={handleCreateGithubRepo}
              disabled={!canManageWorkspace || creatingGithubRepo || !ghRepoName.trim()}
            >
              <FolderGit2 className="h-3.5 w-3.5" />
              {creatingGithubRepo ? "Creating..." : "Create with gh and bind"}
            </Button>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
