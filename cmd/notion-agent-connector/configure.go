package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

func runConfigure(args []string) error {
	fs := flag.NewFlagSet("configure", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		targetFile  string
		accessToken string
		rootPageID  string
	)
	fs.StringVar(&targetFile, "file", ".envrc", "target env file to update")
	fs.StringVar(&accessToken, "token", "", "Notion access token")
	fs.StringVar(&rootPageID, "root-page-id", "", "shared Notion root page id for reads and writes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	token, err := promptRequiredValue(accessToken, "Notion access token: ", true)
	if err != nil {
		return err
	}
	rootPageID, err = promptRequiredValue(rootPageID, "Root page ID: ", false)
	if err != nil {
		return err
	}

	if err := writeConfiguredEnvFile(targetFile, token, rootPageID); err != nil {
		return err
	}

	fmt.Printf("Wrote Notion settings to %s\n", targetFile)
	fmt.Printf("Read root page: %s\n", rootPageID)
	fmt.Printf("Write root page: %s\n", rootPageID)
	if targetFile == ".envrc" {
		fmt.Printf("Run `direnv allow` if this repository uses direnv.\n")
	}
	return nil
}

func promptRequiredValue(currentValue, prompt string, secret bool) (string, error) {
	currentValue = strings.TrimSpace(currentValue)
	if currentValue != "" {
		return currentValue, nil
	}

	fmt.Fprint(os.Stderr, prompt)
	value, err := readPromptValue(secret)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("a value is required")
	}
	return value, nil
}

func readPromptValue(secret bool) (string, error) {
	if secret && term.IsTerminal(int(os.Stdin.Fd())) {
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(value), nil
	}

	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return value, nil
}

func writeConfiguredEnvFile(path, token, rootPageID string) error {
	content, err := buildConfiguredEnvFile(path, token, rootPageID)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, ".notion-agent-connector-config-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func buildConfiguredEnvFile(path, token, rootPageID string) (string, error) {
	existingContent, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	var keptLines []string
	if len(existingContent) > 0 {
		lines := strings.Split(string(existingContent), "\n")
		for _, line := range lines {
			if isManagedNotionEnvLine(line) {
				continue
			}
			keptLines = append(keptLines, line)
		}
	}
	keptLines = trimTrailingEmptyLines(keptLines)

	var builder strings.Builder
	if len(keptLines) > 0 {
		builder.WriteString(strings.Join(keptLines, "\n"))
		builder.WriteString("\n\n")
	}
	builder.WriteString(fmt.Sprintf("export NOTION_AGENT_CONNECTOR_ACCESS_TOKEN=%q\n", token))
	builder.WriteString(fmt.Sprintf("export NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID=%q\n", rootPageID))
	builder.WriteString(fmt.Sprintf("export NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID=%q\n", rootPageID))
	return builder.String(), nil
}

func isManagedNotionEnvLine(line string) bool {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "export ")
	return strings.HasPrefix(line, "NOTION_AGENT_CONNECTOR_ACCESS_TOKEN=") ||
		strings.HasPrefix(line, "NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID=") ||
		strings.HasPrefix(line, "NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID=")
}

func trimTrailingEmptyLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
