package knowledge

import "testing"

func TestHarnessTemplateContainsRequiredSkeleton(t *testing.T) {
	entries := HarnessTemplate()
	result := Validate(entries)
	if !result.Valid {
		t.Fatalf("expected harness template to validate, got errors: %v", result.Errors)
	}
}

func TestValidateReportsMissingRequiredFiles(t *testing.T) {
	entries := []TemplateEntry{
		{Path: "AGENTS.md", Type: EntryTypeFile, Content: "ARCHITECTURE.md docs/"},
		{Path: "docs/README.md", Type: EntryTypeFile, Content: "design-docs/index.md"},
	}
	result := Validate(entries)
	if result.Valid {
		t.Fatal("expected validation to fail when required files are missing")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors, got none")
	}
}

func TestValidateAllowsControlledExtensionsAndBlocksUnknownDocsArea(t *testing.T) {
	entries := HarnessTemplate()
	entries = append(entries, TemplateEntry{
		Path:    "docs/extensions/custom/rules.md",
		Type:    EntryTypeFile,
		Content: "custom extension",
	})
	result := Validate(entries)
	if !result.Valid {
		t.Fatalf("expected extensions path to be allowed, got errors: %v", result.Errors)
	}

	entries = append(entries, TemplateEntry{
		Path:    "docs/random-area/notes.md",
		Type:    EntryTypeFile,
		Content: "should not be allowed",
	})
	result = Validate(entries)
	if result.Valid {
		t.Fatal("expected unknown docs top-level area to fail validation")
	}
}
