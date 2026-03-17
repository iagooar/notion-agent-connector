# Notion Agent Connector

Notion Agent Connector is a Go CLI and companion agent skill for working with Notion at a higher level than raw API calls. It was built to help AI coding agents create, read, and update rich Notion pages with high fidelity, including surgical section edits, native block operations, media uploads, and structured data-source or row workflows.

## Why This Exists

Many agent workflows treat Notion like a markdown bucket: generate a file, overwrite a page, move on. This project takes a different approach. It gives agents:

- a fast local CLI with page, section, block, row, and data-source primitives
- a skill that tells the agent how to choose the right operation for the task
- a high-fidelity write path that uses Notion's markdown APIs when possible and native block or upload flows when needed

The result should feel like real Notion authoring, not a lossy export pipeline.

## What It Can Do

- read a configured root page tree or a single page
- search for pages and discover data sources
- update one heading section without rewriting the whole document
- create or update child pages under a parent
- append, update, or delete exact native blocks
- upload local images, PDFs, audio, video, and files when content requires it
- query data sources and create or update rows directly

## Repository Layout

- `cmd/notion-agent-connector/`: CLI entrypoint and command handlers
- `internal/notion/`: Notion client, markdown parsing, native block sync, and data-source logic
- `internal/config/`: local environment loading
- `scripts/notion-agent-connector.sh`: wrapper that rebuilds `.tmp/bin/notion-agent-connector` when needed
- `.agents/skills/notion-agent-connector/`: canonical skill bundle
- `skills/` and `.claude/`: compatibility mirrors for skill discovery

## Install the CLI

Build the binary:

```bash
go build -o .tmp/bin/notion-agent-connector ./cmd/notion-agent-connector
```

Or use the wrapper script, which rebuilds automatically:

```bash
./scripts/notion-agent-connector.sh capabilities
```

Run the full test suite before relying on local changes:

```bash
go test ./...
```

## Install the Skill

Use `./.agents/skills/notion-agent-connector` as the source of truth. Install it into the agent environment you use:

```bash
mkdir -p ~/.codex/skills ~/.claude/skills ~/.agents/skills
ln -sfn "$PWD/.agents/skills/notion-agent-connector" ~/.codex/skills/notion-agent-connector
ln -sfn "$PWD/.agents/skills/notion-agent-connector" ~/.claude/skills/notion-agent-connector
ln -sfn "$PWD/.agents/skills/notion-agent-connector" ~/.agents/skills/notion-agent-connector
```

Symlinks are recommended so local edits to the skill are picked up immediately.

## Configure Notion

1. Create a Notion internal integration with the content permissions you need.
2. Share a root page with that integration.
3. Run the configure helper:

```bash
.tmp/bin/notion-agent-connector configure
```

You can also pass values directly:

```bash
.tmp/bin/notion-agent-connector configure -token "<secret>" -root-page-id "<page-id>"
```

By default this writes to `.envrc` and sets:

- `NOTION_AGENT_CONNECTOR_ACCESS_TOKEN`
- `NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID`
- `NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID`

If you use direnv, run `direnv allow` after configuration.

## Recommended Agent Workflow

Start by asking the CLI what it supports:

```bash
.tmp/bin/notion-agent-connector capabilities
```

Then prefer the lightest operation that matches the job:

- use `read-root` or `read-page` for normal reading
- use `search` when the target is unknown
- use `write-section` or `write-page -update-section` for precise edits
- use `write-page` for full-page refreshes
- use block commands when exact native Notion structure matters
- use data-source and row commands for structured content instead of page-shaped workarounds

Examples:

```bash
.tmp/bin/notion-agent-connector read-root
.tmp/bin/notion-agent-connector read-page -page-id "<page-id>"
.tmp/bin/notion-agent-connector write-section -page-id "<page-id>" -heading "Status" -source updates.md
.tmp/bin/notion-agent-connector write-page -page-id "<page-id>" -source handbook.md
.tmp/bin/notion-agent-connector query-data-source -data-source-id "<id>"
```

## High-Fidelity Design

The CLI is built to preserve rich Notion behavior. Text-only and remote-media updates stay on Notion's markdown API. When content includes local media or the task needs exact placement, the connector switches to native uploads and block-backed operations. Section-level updates are designed to edit only the target boundary instead of replacing the whole page.

That design is the point of the project: agents should be able to create documentation, updates, to-dos, and operational pages in Notion that still feel native, structured, and editable after the write.
