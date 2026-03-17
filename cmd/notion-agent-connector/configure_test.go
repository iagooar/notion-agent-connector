package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildConfiguredEnvFileForNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".envrc")

	content, err := buildConfiguredEnvFile(path, "secret_test", "page_123")
	if err != nil {
		t.Fatalf("buildConfiguredEnvFile() error = %v", err)
	}

	expected := "" +
		"export NOTION_AGENT_CONNECTOR_ACCESS_TOKEN=\"secret_test\"\n" +
		"export NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID=\"page_123\"\n" +
		"export NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID=\"page_123\"\n"
	if content != expected {
		t.Fatalf("content = %q, want %q", content, expected)
	}
}

func TestBuildConfiguredEnvFilePreservesUnrelatedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".envrc")
	existing := "" +
		"export KEEP_ME=\"1\"\n" +
		"export NOTION_AGENT_CONNECTOR_ACCESS_TOKEN=\"old-token\"\n" +
		"NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID=old-read\n" +
		"export NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID=\"old-write\"\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	content, err := buildConfiguredEnvFile(path, "new-token", "new-page")
	if err != nil {
		t.Fatalf("buildConfiguredEnvFile() error = %v", err)
	}

	expected := "" +
		"export KEEP_ME=\"1\"\n\n" +
		"export NOTION_AGENT_CONNECTOR_ACCESS_TOKEN=\"new-token\"\n" +
		"export NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID=\"new-page\"\n" +
		"export NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID=\"new-page\"\n"
	if content != expected {
		t.Fatalf("content = %q, want %q", content, expected)
	}
}
