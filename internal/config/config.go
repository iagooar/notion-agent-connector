package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	NotionAccessToken string
	NotionBaseURL     string
	WriteRootPageID   string
	ReadRootPageID    string
}

func Load() Config {
	// Prefer .envrc over .env so the recommended direnv-based setup works
	// without needing an interactive shell hook during CLI execution.
	_ = loadDotEnvIfPresent(".envrc")
	_ = loadDotEnvIfPresent(".env")
	return Config{
		NotionAccessToken: strings.TrimSpace(os.Getenv("NOTION_AGENT_CONNECTOR_ACCESS_TOKEN")),
		NotionBaseURL:     strings.TrimSpace(os.Getenv("NOTION_AGENT_CONNECTOR_BASE_URL")),
		WriteRootPageID:   strings.TrimSpace(os.Getenv("NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID")),
		ReadRootPageID:    strings.TrimSpace(os.Getenv("NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID")),
	}
}

func loadDotEnvIfPresent(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		_ = os.Setenv(key, value)
	}
	return scanner.Err()
}
