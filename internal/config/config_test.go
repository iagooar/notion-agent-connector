package config

import (
	"os"
	"testing"
)

func TestLoadReadsScopedEnvVars(t *testing.T) {
	t.Setenv("NOTION_AGENT_CONNECTOR_ACCESS_TOKEN", "token")
	t.Setenv("NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID", "write-root")
	t.Setenv("NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID", "read-root")
	t.Setenv("NOTION_AGENT_CONNECTOR_BASE_URL", "http://localhost:9999/v1")

	cfg := Load()
	if cfg.NotionAccessToken != "token" {
		t.Fatalf("NotionAccessToken = %q", cfg.NotionAccessToken)
	}
	if cfg.WriteRootPageID != "write-root" {
		t.Fatalf("WriteRootPageID = %q", cfg.WriteRootPageID)
	}
	if cfg.ReadRootPageID != "read-root" {
		t.Fatalf("ReadRootPageID = %q", cfg.ReadRootPageID)
	}
	if cfg.NotionBaseURL != "http://localhost:9999/v1" {
		t.Fatalf("NotionBaseURL = %q", cfg.NotionBaseURL)
	}
}

func TestLoadReadsEnvrcFile(t *testing.T) {
	t.Setenv("NOTION_AGENT_CONNECTOR_ACCESS_TOKEN", "")
	t.Setenv("NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID", "")
	t.Setenv("NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID", "")
	t.Setenv("NOTION_AGENT_CONNECTOR_BASE_URL", "")

	tmpDir := t.TempDir()
	envrcPath := tmpDir + "/.envrc"
	if err := os.WriteFile(envrcPath, []byte(`
export NOTION_AGENT_CONNECTOR_ACCESS_TOKEN="envrc-token"
export NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID="envrc-write"
export NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID="envrc-read"
export NOTION_AGENT_CONNECTOR_BASE_URL="http://localhost:4010/v1"
`), 0o644); err != nil {
		t.Fatalf("WriteFile(.envrc) error = %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(wd); chdirErr != nil {
			t.Fatalf("Chdir restore error = %v", chdirErr)
		}
	})
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", tmpDir, err)
	}

	cfg := Load()
	if cfg.NotionAccessToken != "envrc-token" {
		t.Fatalf("NotionAccessToken = %q", cfg.NotionAccessToken)
	}
	if cfg.WriteRootPageID != "envrc-write" {
		t.Fatalf("WriteRootPageID = %q", cfg.WriteRootPageID)
	}
	if cfg.ReadRootPageID != "envrc-read" {
		t.Fatalf("ReadRootPageID = %q", cfg.ReadRootPageID)
	}
	if cfg.NotionBaseURL != "http://localhost:4010/v1" {
		t.Fatalf("NotionBaseURL = %q", cfg.NotionBaseURL)
	}
}

func TestLoadPrefersEnvrcOverEnv(t *testing.T) {
	t.Setenv("NOTION_AGENT_CONNECTOR_ACCESS_TOKEN", "")
	t.Setenv("NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID", "")
	t.Setenv("NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID", "")

	tmpDir := t.TempDir()
	if err := os.WriteFile(tmpDir+"/.envrc", []byte(`
export NOTION_AGENT_CONNECTOR_ACCESS_TOKEN="envrc-token"
export NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID="envrc-write"
export NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID="envrc-read"
`), 0o644); err != nil {
		t.Fatalf("WriteFile(.envrc) error = %v", err)
	}
	if err := os.WriteFile(tmpDir+"/.env", []byte(`
NOTION_AGENT_CONNECTOR_ACCESS_TOKEN=dotenv-token
NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID=dotenv-write
NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID=dotenv-read
`), 0o644); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(wd); chdirErr != nil {
			t.Fatalf("Chdir restore error = %v", chdirErr)
		}
	})
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", tmpDir, err)
	}

	cfg := Load()
	if cfg.NotionAccessToken != "envrc-token" {
		t.Fatalf("NotionAccessToken = %q", cfg.NotionAccessToken)
	}
	if cfg.WriteRootPageID != "envrc-write" {
		t.Fatalf("WriteRootPageID = %q", cfg.WriteRootPageID)
	}
	if cfg.ReadRootPageID != "envrc-read" {
		t.Fatalf("ReadRootPageID = %q", cfg.ReadRootPageID)
	}
}
