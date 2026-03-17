# Repository Guidelines

## Product Intent
This project exists to give AI coding agents a better way to work with Notion. Contributors should preserve the core idea: provide a CLI with the right abstraction level plus a companion skill that teaches agents when to read, write, update sections, use native blocks, and use structured row or data-source operations. The goal is high-fidelity Notion pages, not markdown dumps.

## Project Structure & Module Organization
`cmd/notion-agent-connector/` contains the CLI entrypoint and subcommand wiring. Keep user-facing flags and command flow here. `internal/notion/` contains the Notion client, markdown parsing, native block sync, media upload handling, and data-source logic. `internal/config/` loads local config from `.envrc` first, then `.env`.

Tests live next to the code as `*_test.go`. `scripts/notion-agent-connector.sh` rebuilds the cached binary in `.tmp/bin/` and is the easiest local entrypoint. Treat `./.agents/skills/notion-agent-connector` as the canonical skill bundle; `./skills/` and `./.claude/` are compatibility mirrors.

## Build, Test, and Development Commands
Run `go test ./...` for the full suite. Use `go test ./internal/notion -run UpdateDocumentSection` for focused document-sync work. Build with `go build ./cmd/notion-agent-connector`. Run the tool with `./scripts/notion-agent-connector.sh capabilities` or another subcommand to rebuild on demand. Use `go test -cover ./...` before larger changes.

## Coding Style & Naming Conventions
Format with `gofmt`; standard Go tabs and layout apply. Use `PascalCase` for exported names, `camelCase` for internal helpers, and keep `cmd/...` thin by moving reusable logic into `internal/...`. Follow the existing `NOTION_AGENT_CONNECTOR_*` environment variable prefix.

## Testing Guidelines
Use Go's `testing` package, `httptest` for fake Notion APIs, and `t.TempDir()` for file-based cases. Add tests for new flags, config behavior, markdown transforms, native block flows, and surgical section updates. Keep tests deterministic and avoid live Notion calls.

## Commit & Pull Request Guidelines
This repo currently has no local commit history, so use short imperative commit subjects such as `add row property pagination`. PRs should explain the user-facing change, include test evidence, show example CLI usage for new behavior, and call out any new env vars, permissions, or skill-installation changes.
