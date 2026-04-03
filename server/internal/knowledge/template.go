package knowledge

import (
	"fmt"
	"path/filepath"
	"strings"
)

const TemplateVersion = "openai-harness-v1"

type EntryType string

const (
	EntryTypeFile EntryType = "file"
	EntryTypeDir  EntryType = "dir"
)

type TemplateEntry struct {
	Path    string    `json:"path"`
	Type    EntryType `json:"type"`
	Content string    `json:"content,omitempty"`
}

func HarnessTemplate() []TemplateEntry {
	return []TemplateEntry{
		{Path: "AGENTS.md", Type: EntryTypeFile, Content: agentsTemplate},
		{Path: "ARCHITECTURE.md", Type: EntryTypeFile, Content: architectureTemplate},

		{Path: "docs", Type: EntryTypeDir},
		{Path: "docs/README.md", Type: EntryTypeFile, Content: docsReadmeTemplate},
		{Path: "docs/CONTRIBUTING.md", Type: EntryTypeFile, Content: docsContributingTemplate},
		{Path: "docs/TERMS.md", Type: EntryTypeFile, Content: docsTermsTemplate},
		{Path: "docs/SECURITY.md", Type: EntryTypeFile, Content: docsSecurityTemplate},

		{Path: "docs/design-docs", Type: EntryTypeDir},
		{Path: "docs/design-docs/index.md", Type: EntryTypeFile, Content: designDocsIndexTemplate},

		{Path: "docs/exec-plans", Type: EntryTypeDir},
		{Path: "docs/exec-plans/index.md", Type: EntryTypeFile, Content: execPlansIndexTemplate},

		{Path: "docs/generated", Type: EntryTypeDir},
		{Path: "docs/generated/index.md", Type: EntryTypeFile, Content: generatedIndexTemplate},

		{Path: "docs/product-specs", Type: EntryTypeDir},
		{Path: "docs/product-specs/index.md", Type: EntryTypeFile, Content: productSpecsIndexTemplate},

		{Path: "docs/references", Type: EntryTypeDir},
		{Path: "docs/references/index.md", Type: EntryTypeFile, Content: referencesIndexTemplate},
		{Path: "docs/references/openai", Type: EntryTypeDir},
		{Path: "docs/references/openai/harness-engineering.md", Type: EntryTypeFile, Content: harnessReferenceTemplate},

		{Path: "docs/extensions", Type: EntryTypeDir},
	}
}

type ValidateResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func Validate(entries []TemplateEntry) ValidateResult {
	requiredFiles := map[string]bool{
		"AGENTS.md":                   true,
		"ARCHITECTURE.md":             true,
		"docs/README.md":              true,
		"docs/CONTRIBUTING.md":        true,
		"docs/TERMS.md":               true,
		"docs/SECURITY.md":            true,
		"docs/design-docs/index.md":   true,
		"docs/exec-plans/index.md":    true,
		"docs/generated/index.md":     true,
		"docs/product-specs/index.md": true,
		"docs/references/index.md":    true,
	}
	requiredDirs := []string{
		"docs/design-docs",
		"docs/exec-plans",
		"docs/generated",
		"docs/product-specs",
		"docs/references",
	}
	allowedDocsTopLevel := map[string]bool{
		"README.md":       true,
		"CONTRIBUTING.md": true,
		"TERMS.md":        true,
		"SECURITY.md":     true,
		"design-docs":     true,
		"exec-plans":      true,
		"generated":       true,
		"product-specs":   true,
		"references":      true,
		"extensions":      true,
	}

	existingFiles := map[string]TemplateEntry{}
	existingDirs := map[string]bool{}

	for _, entry := range entries {
		path := strings.TrimSpace(strings.TrimPrefix(filepath.Clean(entry.Path), "./"))
		if path == "." || path == "" {
			continue
		}

		if entry.Type == EntryTypeDir {
			existingDirs[path] = true
			continue
		}

		existingFiles[path] = entry
		existingDirs[filepath.Dir(path)] = true
	}

	var errs []string
	var warns []string

	for path := range requiredFiles {
		if _, ok := existingFiles[path]; !ok {
			errs = append(errs, fmt.Sprintf("missing required file: %s", path))
		}
	}

	for _, dir := range requiredDirs {
		found := existingDirs[dir]
		if !found {
			for filePath := range existingFiles {
				if strings.HasPrefix(filePath, dir+"/") {
					found = true
					break
				}
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("missing required directory: %s", dir))
		}
	}

	for filePath := range existingFiles {
		if !strings.HasPrefix(filePath, "docs/") {
			continue
		}
		rest := strings.TrimPrefix(filePath, "docs/")
		if rest == "" {
			continue
		}
		first := strings.Split(rest, "/")[0]
		if !allowedDocsTopLevel[first] {
			errs = append(errs, fmt.Sprintf("docs top-level area %q is not allowed; use docs/extensions/ for custom sections", first))
		}
	}

	if agents, ok := existingFiles["AGENTS.md"]; ok {
		if agents.Content == "" {
			warns = append(warns, "AGENTS.md content missing; cannot validate cross-links")
		} else {
			if !strings.Contains(agents.Content, "ARCHITECTURE.md") {
				errs = append(errs, "AGENTS.md must link to ARCHITECTURE.md")
			}
			if !strings.Contains(agents.Content, "docs/") {
				errs = append(errs, "AGENTS.md should reference the docs/ directory")
			}
		}
	}

	if arch, ok := existingFiles["ARCHITECTURE.md"]; ok {
		if arch.Content == "" {
			warns = append(warns, "ARCHITECTURE.md content missing; cannot validate references")
		} else if !strings.Contains(arch.Content, "docs/") {
			errs = append(errs, "ARCHITECTURE.md should reference docs/ indexes")
		}
	}

	if docsReadme, ok := existingFiles["docs/README.md"]; ok {
		if docsReadme.Content == "" {
			warns = append(warns, "docs/README.md content missing; cannot validate index links")
		} else {
			expectedRefs := []string{
				"design-docs/index.md",
				"exec-plans/index.md",
				"generated/index.md",
				"product-specs/index.md",
				"references/index.md",
			}
			for _, ref := range expectedRefs {
				if !strings.Contains(docsReadme.Content, ref) {
					errs = append(errs, fmt.Sprintf("docs/README.md should link to %s", ref))
				}
			}
		}
	}

	return ValidateResult{
		Valid:    len(errs) == 0,
		Errors:   errs,
		Warnings: warns,
	}
}

const agentsTemplate = `# Agents

See [ARCHITECTURE.md](./ARCHITECTURE.md) for system boundaries and design constraints.

Knowledge is organized under [docs/](./docs/README.md). Agents should update only the appropriate section and keep indexes current.
`

const architectureTemplate = `# Architecture

This document describes system boundaries, data flow, and major components.

Keep implementation-level details in the docs indexes:
- [Design Docs](./docs/design-docs/index.md)
- [Execution Plans](./docs/exec-plans/index.md)
- [Generated Docs](./docs/generated/index.md)
- [Product Specs](./docs/product-specs/index.md)
- [References](./docs/references/index.md)
`

const docsReadmeTemplate = `# Docs Index

- [Design Docs](./design-docs/index.md)
- [Execution Plans](./exec-plans/index.md)
- [Generated Docs](./generated/index.md)
- [Product Specs](./product-specs/index.md)
- [References](./references/index.md)
- [Contributing](./CONTRIBUTING.md)
- [Terms](./TERMS.md)
- [Security](./SECURITY.md)
`

const docsContributingTemplate = `# Contributing

1. Keep entries concise and factual.
2. Update the corresponding index when adding new docs.
3. Prefer links over duplication.
`

const docsTermsTemplate = `# Terms

Define shared project and domain vocabulary here.
`

const docsSecurityTemplate = `# Security

Document threat model, controls, incident procedures, and reporting channels.
`

const designDocsIndexTemplate = `# Design Docs

Catalog technical design documents and ADRs.
`

const execPlansIndexTemplate = `# Execution Plans

Track implementation plans and rollout checklists.
`

const generatedIndexTemplate = `# Generated Docs

Store generated artifacts and machine-produced summaries.
`

const productSpecsIndexTemplate = `# Product Specs

Track PRDs, feature specs, and product requirements.
`

const referencesIndexTemplate = `# References

- [OpenAI Harness Engineering](./openai/harness-engineering.md)
`

const harnessReferenceTemplate = `# Harness Engineering

Reference: https://openai.com/index/harness-engineering
`
