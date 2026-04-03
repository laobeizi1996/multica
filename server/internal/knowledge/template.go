package knowledge

import (
	"fmt"
	"path/filepath"
	"strings"
)

const TemplateVersion = "openai-harness-v2"

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
		{Path: "docs/DESIGN.md", Type: EntryTypeFile, Content: docsDesignTemplate},
		{Path: "docs/FRONTEND.md", Type: EntryTypeFile, Content: docsFrontendTemplate},
		{Path: "docs/GLOSSARY.md", Type: EntryTypeFile, Content: docsGlossaryTemplate},
		{Path: "docs/PLANS.md", Type: EntryTypeFile, Content: docsPlansTemplate},
		{Path: "docs/PRODUCT_SENSE.md", Type: EntryTypeFile, Content: docsProductSenseTemplate},
		{Path: "docs/QUALITY_SCORE.md", Type: EntryTypeFile, Content: docsQualityScoreTemplate},
		{Path: "docs/RELIABILITY.md", Type: EntryTypeFile, Content: docsReliabilityTemplate},
		{Path: "docs/TERMS.md", Type: EntryTypeFile, Content: docsTermsTemplate},
		{Path: "docs/SECURITY.md", Type: EntryTypeFile, Content: docsSecurityTemplate},

		{Path: "docs/design-docs", Type: EntryTypeDir},
		{Path: "docs/design-docs/design-doc1.md", Type: EntryTypeFile, Content: designDoc1Template},
		{Path: "docs/design-docs/index.md", Type: EntryTypeFile, Content: designDocsIndexTemplate},

		{Path: "docs/exec-plans", Type: EntryTypeDir},
		{Path: "docs/exec-plans/active", Type: EntryTypeDir},
		{Path: "docs/exec-plans/active/plan1.md", Type: EntryTypeFile, Content: activePlanTemplate},
		{Path: "docs/exec-plans/completed", Type: EntryTypeDir},
		{Path: "docs/exec-plans/completed/plan2.md", Type: EntryTypeFile, Content: completedPlanTemplate},
		{Path: "docs/exec-plans/index.md", Type: EntryTypeFile, Content: execPlansIndexTemplate},

		{Path: "docs/generated", Type: EntryTypeDir},
		{Path: "docs/generated/db-schema.md", Type: EntryTypeFile, Content: generatedDbSchemaTemplate},
		{Path: "docs/generated/index.md", Type: EntryTypeFile, Content: generatedIndexTemplate},

		{Path: "docs/product-specs", Type: EntryTypeDir},
		{Path: "docs/product-specs/active", Type: EntryTypeDir},
		{Path: "docs/product-specs/active/spec1.md", Type: EntryTypeFile, Content: activeSpecTemplate},
		{Path: "docs/product-specs/completed", Type: EntryTypeDir},
		{Path: "docs/product-specs/completed/spec2.md", Type: EntryTypeFile, Content: completedSpecTemplate},
		{Path: "docs/product-specs/index.md", Type: EntryTypeFile, Content: productSpecsIndexTemplate},

		{Path: "docs/references", Type: EntryTypeDir},
		{Path: "docs/references/active", Type: EntryTypeDir},
		{Path: "docs/references/active/reference1.md", Type: EntryTypeFile, Content: activeReferenceTemplate},
		{Path: "docs/references/completed", Type: EntryTypeDir},
		{Path: "docs/references/completed/reference2.md", Type: EntryTypeFile, Content: completedReferenceTemplate},
		{Path: "docs/references/index.md", Type: EntryTypeFile, Content: referencesIndexTemplate},
		{Path: "docs/references/llms-full.txt", Type: EntryTypeFile, Content: llmsFullTemplate},
		{Path: "docs/references/llms-small.txt", Type: EntryTypeFile, Content: llmsSmallTemplate},
	}
}

type ValidateResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func Validate(entries []TemplateEntry) ValidateResult {
	requiredFiles := map[string]bool{
		"AGENTS.md":                               true,
		"ARCHITECTURE.md":                         true,
		"docs/README.md":                          true,
		"docs/CONTRIBUTING.md":                    true,
		"docs/DESIGN.md":                          true,
		"docs/FRONTEND.md":                        true,
		"docs/GLOSSARY.md":                        true,
		"docs/PLANS.md":                           true,
		"docs/PRODUCT_SENSE.md":                   true,
		"docs/QUALITY_SCORE.md":                   true,
		"docs/RELIABILITY.md":                     true,
		"docs/TERMS.md":                           true,
		"docs/SECURITY.md":                        true,
		"docs/design-docs/design-doc1.md":         true,
		"docs/design-docs/index.md":               true,
		"docs/exec-plans/active/plan1.md":         true,
		"docs/exec-plans/completed/plan2.md":      true,
		"docs/exec-plans/index.md":                true,
		"docs/generated/db-schema.md":             true,
		"docs/generated/index.md":                 true,
		"docs/product-specs/active/spec1.md":      true,
		"docs/product-specs/completed/spec2.md":   true,
		"docs/product-specs/index.md":             true,
		"docs/references/active/reference1.md":    true,
		"docs/references/completed/reference2.md": true,
		"docs/references/index.md":                true,
		"docs/references/llms-full.txt":           true,
		"docs/references/llms-small.txt":          true,
	}
	requiredDirs := []string{
		"docs/design-docs",
		"docs/exec-plans",
		"docs/exec-plans/active",
		"docs/exec-plans/completed",
		"docs/generated",
		"docs/product-specs",
		"docs/product-specs/active",
		"docs/product-specs/completed",
		"docs/references",
		"docs/references/active",
		"docs/references/completed",
	}
	allowedDocsTopLevel := map[string]bool{
		"README.md":        true,
		"CONTRIBUTING.md":  true,
		"DESIGN.md":        true,
		"FRONTEND.md":      true,
		"GLOSSARY.md":      true,
		"PLANS.md":         true,
		"PRODUCT_SENSE.md": true,
		"QUALITY_SCORE.md": true,
		"RELIABILITY.md":   true,
		"TERMS.md":         true,
		"SECURITY.md":      true,
		"design-docs":      true,
		"exec-plans":       true,
		"generated":        true,
		"product-specs":    true,
		"references":       true,
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
			errs = append(errs, fmt.Sprintf("docs top-level area %q is not allowed", first))
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
				"DESIGN.md",
				"FRONTEND.md",
				"GLOSSARY.md",
				"PLANS.md",
				"PRODUCT_SENSE.md",
				"QUALITY_SCORE.md",
				"RELIABILITY.md",
				"SECURITY.md",
				"TERMS.md",
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

Agents should:

1. Start from [ARCHITECTURE.md](./ARCHITECTURE.md) before changing system behavior.
2. Store durable knowledge in [docs/](./docs/README.md) using the existing sections.
3. Keep directory indexes updated when adding or moving files.
`

const architectureTemplate = `# Architecture

## System Boundaries

Describe core services, ownership boundaries, and runtime assumptions.

## Knowledge System

The documentation system follows the Harness-style structure under [docs/](./docs/README.md):

- [Design Docs](./docs/design-docs/index.md)
- [Execution Plans](./docs/exec-plans/index.md)
- [Generated Docs](./docs/generated/index.md)
- [Product Specs](./docs/product-specs/index.md)
- [References](./docs/references/index.md)
`

const docsReadmeTemplate = `# Docs Index

- [Contributing](./CONTRIBUTING.md)
- [Design](./DESIGN.md)
- [Frontend](./FRONTEND.md)
- [Glossary](./GLOSSARY.md)
- [Plans](./PLANS.md)
- [Product Sense](./PRODUCT_SENSE.md)
- [Quality Score](./QUALITY_SCORE.md)
- [Reliability](./RELIABILITY.md)
- [Security](./SECURITY.md)
- [Terms](./TERMS.md)
- [Design Docs](./design-docs/index.md)
- [Execution Plans](./exec-plans/index.md)
- [Generated Docs](./generated/index.md)
- [Product Specs](./product-specs/index.md)
- [References](./references/index.md)
`

const docsContributingTemplate = `# Contributing

1. Keep entries concise and factual.
2. Update the corresponding index when adding new docs.
3. Prefer links over duplication.
`

const docsDesignTemplate = `# Design

Capture architecture decisions, subsystem boundaries, and major tradeoffs.
For deep technical proposals, use [design-docs/](./design-docs/index.md).
`

const docsFrontendTemplate = `# Frontend

Document frontend principles, design system usage, and interaction standards.
`

const docsGlossaryTemplate = `# Glossary

Define product and technical terms shared across teams.
`

const docsPlansTemplate = `# Plans

Explain planning conventions and link to active execution plans:
- [Execution Plans](./exec-plans/index.md)
`

const docsProductSenseTemplate = `# Product Sense

Summarize user problems, expected outcomes, and prioritization principles.
`

const docsQualityScoreTemplate = `# Quality Score

Track quality dimensions such as correctness, usability, reliability, and velocity.
`

const docsReliabilityTemplate = `# Reliability

Document SLOs, incidents, and resilience improvement strategy.
`

const docsTermsTemplate = `# Terms

Define shared project and domain vocabulary here.
`

const docsSecurityTemplate = `# Security

Document threat model, controls, incident procedures, and reporting channels.
`

const designDoc1Template = `# Design Doc 1

## Context

## Proposal

## Tradeoffs

## Rollout
`

const designDocsIndexTemplate = `# Design Docs

Catalog technical design documents and ADRs.

- [Design Doc 1](./design-doc1.md)
`

const execPlansIndexTemplate = `# Execution Plans

## Active

- [Plan 1](./active/plan1.md)

## Completed

- [Plan 2](./completed/plan2.md)
`

const generatedIndexTemplate = `# Generated Docs

Store generated artifacts and machine-produced summaries.

- [DB Schema](./db-schema.md)
`

const productSpecsIndexTemplate = `# Product Specs

## Active

- [Spec 1](./active/spec1.md)

## Completed

- [Spec 2](./completed/spec2.md)
`

const referencesIndexTemplate = `# References

## Active

- [Reference 1](./active/reference1.md)

## Completed

- [Reference 2](./completed/reference2.md)

## LLM Context Files

- [llms-full.txt](./llms-full.txt)
- [llms-small.txt](./llms-small.txt)
`

const activePlanTemplate = `# Plan 1

## Goal

## Scope

## Milestones
`

const completedPlanTemplate = `# Plan 2 (Completed)

## Outcome

## Lessons Learned
`

const generatedDbSchemaTemplate = `# DB Schema

Generated database schema snapshot and notes.
`

const activeSpecTemplate = `# Spec 1

## Problem

## User Story

## Acceptance Criteria
`

const completedSpecTemplate = `# Spec 2 (Completed)

## Summary

## Validation
`

const activeReferenceTemplate = `# Reference 1

Summary of an actively used external/internal reference.
`

const completedReferenceTemplate = `# Reference 2 (Archived)

Reference retained for historical context.
`

const llmsFullTemplate = `# LLMs Full Context

Reference: https://openai.com/index/harness-engineering
`

const llmsSmallTemplate = `# LLMs Small Context

Reference: https://openai.com/index/harness-engineering
`
