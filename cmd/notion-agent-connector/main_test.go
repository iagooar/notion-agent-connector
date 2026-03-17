package main

import (
	"path/filepath"
	"strings"
	"testing"

	"notion-agent-connector/internal/notion"
)

func TestBuildWritePlanUsesMarkdownReplaceByDefault(t *testing.T) {
	doc, err := notion.ParseMarkdown("# Manual\n\nFresh content")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	plan := buildWritePlan(doc, false, false)
	if plan.Strategy != "markdown replace_content" {
		t.Fatalf("Strategy = %q", plan.Strategy)
	}
	if plan.LocalUploadCount != 0 {
		t.Fatalf("LocalUploadCount = %d", plan.LocalUploadCount)
	}
}

func TestBuildWritePlanPrefersNativeBlockSyncWhenLocalMediaExists(t *testing.T) {
	doc, err := notion.ParseMarkdown("# Manual\n\n![Setup](assets/setup.png)\n\n<pdf src=\"docs/spec.pdf\">Spec</pdf>")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	plan := buildWritePlan(doc, false, false)
	if plan.Strategy != "native block sync with append-then-archive because local media uploads are present" {
		t.Fatalf("Strategy = %q", plan.Strategy)
	}
	if plan.LocalUploadCount != 2 {
		t.Fatalf("LocalUploadCount = %d", plan.LocalUploadCount)
	}
	if got := len(plan.LocalUploadByKind); got != 2 {
		t.Fatalf("len(LocalUploadByKind) = %d", got)
	}
	if plan.LocalUploadByKind[0] != "image=1" || plan.LocalUploadByKind[1] != "pdf=1" {
		t.Fatalf("LocalUploadByKind = %#v", plan.LocalUploadByKind)
	}
}

func TestBuildWritePlanDescribesSectionUpdateForChildPages(t *testing.T) {
	doc, err := notion.ParseMarkdown("# Manual\n\n## Setup\n\nFresh content")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	plan := buildWritePlan(doc, true, true)
	if plan.Strategy != "markdown update_content for an existing child page, or markdown create for a missing child page" {
		t.Fatalf("Strategy = %q", plan.Strategy)
	}
}

func TestBuildWritePlanUsesSectionSwapForLocalMediaUpdates(t *testing.T) {
	doc, err := notion.ParseMarkdown("# Manual\n\n## Setup\n\nFresh content\n\n![Screenshot](assets/setup.png)")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	plan := buildWritePlan(doc, false, true)
	if plan.Strategy != "surgical section block swap with local media uploads" {
		t.Fatalf("Strategy = %q", plan.Strategy)
	}
}

func TestValidateLocalAssetsExplainsSourceRelativeLookup(t *testing.T) {
	doc, err := notion.ParseMarkdown("# Manual\n\n![Setup](../assets/setup.png)")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	notion.ResolveRelativeAssetPaths(&doc, filepath.Join("docs", "generated"))

	err = validateLocalAssets(doc, filepath.Join("docs", "generated", "smoke.md"))
	if err == nil {
		t.Fatal("expected missing asset error")
	}
	if !strings.Contains(err.Error(), "relative paths are resolved from docs/generated") {
		t.Fatalf("unexpected error = %v", err)
	}
}
