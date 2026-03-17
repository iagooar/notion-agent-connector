---
name: notion-agent-connector
description: Read and write arbitrary Notion content through the shared connector CLI, including page trees, native block operations, surgical section updates, native media uploads, search, and Notion data source or row operations. Use when Codex needs to configure a local Notion connection, inspect existing content, publish markdown, refresh one section precisely, or work with database-like Notion data without hand-building raw API calls.
---

# Notion Agent Connector

Use this skill to work with generic Notion content through the compiled `notion-agent-connector` CLI in this repository.

Read [`references/notion-agent-connector.md`](./references/notion-agent-connector.md) first.

## Install

After cloning this repository, install the canonical skill bundle from `./.agents/skills/notion-agent-connector`.

- copy that folder into `~/.codex/skills/notion-agent-connector`, `~/.agents/skills/notion-agent-connector`, or `~/.claude/skills/notion-agent-connector`
- use a symlink instead of a copy when you want local skill edits in this repo to show up immediately in your user skill directory
- treat `./.agents/skills/notion-agent-connector` as the source of truth; `./.claude/skills/notion-agent-connector` and `./skills/notion-agent-connector` exist for discovery and compatibility

## Workflow

1. Build the CLI once before repeated use.

```bash
go build -o .tmp/bin/notion-agent-connector ./cmd/notion-agent-connector
```

2. Check the command surface before guessing.

Run:

```bash
.tmp/bin/notion-agent-connector capabilities
```

That command is the quickest way for an agent to see the current read, write, and data-source primitives without knowing the internal implementation.

3. Configure the repository only from the current execution context.

- trust the current process environment
- trust `.envrc` or `.env` only in the current working directory because this CLI loads those files
- do not inspect shell profiles, sibling repositories, parent directories, or unrelated project folders unless the user explicitly asks
- if the required vars are not available in the current environment or current directory, treat the skill as unconfigured and ask the user to configure it

Run:

```bash
.tmp/bin/notion-agent-connector configure
```

Ask only for:

- the Notion internal integration token
- the root page id where the skill should work

The configure helper writes the same page id to both `NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID` and `NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID` in `.envrc` by default. Override `-root-page-id`, `-parent-page-id`, or `-page-id` per command only when the task clearly needs a different target.

4. Prefer the lightest read primitive that answers the task.

- use `.tmp/bin/notion-agent-connector read-root` for a configured page tree
- use `.tmp/bin/notion-agent-connector read-page -page-id "<page-id>"` for one specific page
- use `.tmp/bin/notion-agent-connector get-block -block-id "<block-id>"` or `.tmp/bin/notion-agent-connector list-block-children -block-id "<block-id>"` when the page has native block structure you want to inspect directly
- use `.tmp/bin/notion-agent-connector search -query "<text>"` for workspace discovery or `.tmp/bin/notion-agent-connector search -root-page-id "<page-id>" -query "<text>"` for root-scoped discovery
- use `.tmp/bin/notion-agent-connector get-database`, `list-data-sources`, `get-data-source`, `query-data-source`, `get-row`, or `get-row-property` for structured Notion data

Keep markdown mode as the default path. Use `-read-mode blocks` only when you intentionally want the block-shaped fallback view or when markdown is incomplete.

If a markdown read still reports incomplete content after `unknown_block_ids` recovery:

- treat the markdown output as partial
- rerun with `-read-mode blocks` when you need the fallback view
- do not summarize the partial markdown as if it were the whole page

Use `-include-transcript` when reading meeting notes and transcript text matters to the task.

5. Prefer surgical writes over full rewrites.

- use `.tmp/bin/notion-agent-connector write-section -page-id "<page-id>" -heading "<heading>" -source <path>` or `.tmp/bin/notion-agent-connector write-page -page-id "<page-id>" -update-section "<heading>" -source <path>` when one stable section changed
- use `.tmp/bin/notion-agent-connector write-page -source <path>` only when you intentionally want to refresh the whole page body
- use `.tmp/bin/notion-agent-connector write-page -as-child-page -parent-page-id "<page-id>" -source <path> -title "<title>"` for a child page flow
- use `.tmp/bin/notion-agent-connector append-block-children`, `update-block`, or `delete-block` when you need to preserve native Notion structure and operate on exact blocks instead of rewriting larger sections

The CLI prefers Notion's markdown API for text-only and remote-media content, and switches automatically to native upload plus block sync only when local media requires it.

6. Use the database and row primitives directly instead of forcing page-shaped workarounds.

- discover child databases and data sources with `list-data-sources`
- inspect a database with `get-database`
- inspect schema with `get-data-source`
- query rows with `query-data-source`
- create rows with `create-row`
- update row properties with `update-row-properties`
- inspect rows with `get-row` and `get-row-property`
- update data-source title, description, or schema with `update-data-source`

Row bodies are still Notion pages, so once you know a row page id you can use `read-page`, `write-page`, and `write-section` on that row body.

7. Keep the content grounded and high-fidelity.

- draft markdown locally first, then publish it
- prefer `-dry-run` before a write when the target page, parent, or section boundary is uncertain
- use markdown mode whenever possible for the highest Notion fidelity
- use block commands when the task is really block-shaped, for example native embeds, bookmarks, columns, breadcrumbs, or table-of-contents markers
- use section writes for precise updates and row property commands for structured fields
- when a URL should behave like a link in Notion, write it as a clickable markdown link such as `[label](https://example.com)` or as native linked rich text through block commands; do not leave user-facing links as plain text or inline code unless there is a strong task-specific reason
- remember that file-backed URLs returned by Notion reads are temporary signed URLs
- use `get-row-property -page-size ... -cursor ...` when property items are large enough to paginate
- treat database references and database mentions inside markdown as unstable round-trips for now; use the explicit data-source and row commands instead
- do not encourage comment commands in the normal workflow for now; many integrations lack the required Notion permission even when read and write content access is configured
