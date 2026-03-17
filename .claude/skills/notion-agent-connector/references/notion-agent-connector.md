# Notion Agent Connector

## Required env vars

- `NOTION_AGENT_CONNECTOR_ACCESS_TOKEN`
- `NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID`
- `NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID`
- `NOTION_AGENT_CONNECTOR_BASE_URL` optional for tests or mocks

The configure helper keeps setup minimal by writing the same page id to both the read and write roots unless you override one of them later.

## Install from this repository

Use the canonical bundle in `./.agents/skills/notion-agent-connector`.

- copy that folder into `~/.codex/skills/notion-agent-connector`, `~/.agents/skills/notion-agent-connector`, or `~/.claude/skills/notion-agent-connector`
- use a symlink if you want local changes in this repository to update the installed skill automatically
- treat `./.agents/skills/notion-agent-connector` as the source of truth
- treat `./.claude/skills/notion-agent-connector` and `./skills/notion-agent-connector` as wrappers or compatibility mirrors

## Config discovery boundary

- check the live process environment first
- check `.envrc` or `.env` only in the current working directory
- do not inspect other repositories, sibling folders, parent folders, shell profiles, or unrelated env files unless the user explicitly asks
- if the vars are missing in the current environment and current directory, treat the skill as unconfigured

## Build once

```bash
go build -o .tmp/bin/notion-agent-connector ./cmd/notion-agent-connector
```

To see the current command surface:

```bash
.tmp/bin/notion-agent-connector capabilities
```

## Configure

```bash
.tmp/bin/notion-agent-connector configure
.tmp/bin/notion-agent-connector configure -token "<secret>" -root-page-id "<page-id>"
.tmp/bin/notion-agent-connector configure -file .env
```

## Read commands

Page-tree and page-body reads:

```bash
.tmp/bin/notion-agent-connector read-root
.tmp/bin/notion-agent-connector read-root -root-page-id "<page-id>"
.tmp/bin/notion-agent-connector read-root -format json
.tmp/bin/notion-agent-connector read-root -max-depth 2
.tmp/bin/notion-agent-connector read-root -read-mode blocks
.tmp/bin/notion-agent-connector read-root -include-transcript
.tmp/bin/notion-agent-connector read-page -page-id "<page-id>"
.tmp/bin/notion-agent-connector read-page -page-id "<page-id>" -read-mode blocks
```

Database and row reads:

```bash
.tmp/bin/notion-agent-connector get-block -block-id "<block-id>"
.tmp/bin/notion-agent-connector list-block-children -block-id "<block-id>"
.tmp/bin/notion-agent-connector list-block-children -block-id "<block-id>" -recursive -format markdown
.tmp/bin/notion-agent-connector search -query "Connector Verification"
.tmp/bin/notion-agent-connector search -root-page-id "<page-id>" -query "Connector Verification"
.tmp/bin/notion-agent-connector list-data-sources
.tmp/bin/notion-agent-connector list-data-sources -page-id "<page-id>"
.tmp/bin/notion-agent-connector get-database -database-id "<database-id>"
.tmp/bin/notion-agent-connector get-data-source -data-source-id "<data-source-id>"
.tmp/bin/notion-agent-connector query-data-source -data-source-id "<data-source-id>"
.tmp/bin/notion-agent-connector query-data-source -data-source-id "<data-source-id>" -title-contains "Connector"
.tmp/bin/notion-agent-connector query-data-source -data-source-id "<data-source-id>" -filter-json '{"property":"Status","status":{"equals":"Active"}}'
.tmp/bin/notion-agent-connector query-data-source -data-source-id "<data-source-id>" -sort-json '[{"timestamp":"last_edited_time","direction":"descending"}]'
.tmp/bin/notion-agent-connector query-data-source -data-source-id "<data-source-id>" -page-size 20 -cursor "<cursor>"
.tmp/bin/notion-agent-connector get-row -page-id "<row-page-id>"
.tmp/bin/notion-agent-connector get-row-property -page-id "<row-page-id>" -property "Status"
```

## Write commands

Page-body writes:

```bash
.tmp/bin/notion-agent-connector write-page -source <path> -title "<page title>"
.tmp/bin/notion-agent-connector write-page -dry-run -source <path> -title "<page title>"
.tmp/bin/notion-agent-connector write-page -page-id "<page-id>" -source <path>
.tmp/bin/notion-agent-connector write-page -page-id "<page-id>" -update-section "<heading>" -source <path>
.tmp/bin/notion-agent-connector write-section -page-id "<page-id>" -heading "<heading>" -source <path>
.tmp/bin/notion-agent-connector write-page -as-child-page -parent-page-id "<page-id>" -source <path> -title "<page title>"
```

Structured data writes:

```bash
.tmp/bin/notion-agent-connector append-block-children -block-id "<block-id>" -children-json @blocks.json
.tmp/bin/notion-agent-connector update-block -block-id "<block-id>" -block-json @block.json
.tmp/bin/notion-agent-connector delete-block -block-id "<block-id>"
.tmp/bin/notion-agent-connector create-row -data-source-id "<data-source-id>" -properties-json @row.json
.tmp/bin/notion-agent-connector update-row-properties -page-id "<row-page-id>" -properties-json @row.json
.tmp/bin/notion-agent-connector update-data-source -data-source-id "<data-source-id>" -title "Connector Verification"
.tmp/bin/notion-agent-connector update-data-source -data-source-id "<data-source-id>" -properties-json @data-source-properties.json
```

## Recommended workflow

### 1. Default to markdown reads and writes

- prefer `read-root` or `read-page` in markdown mode first
- prefer `write-page` or `write-section` for text-only and remote-media content
- use `-read-mode blocks` only when markdown is incomplete or when you intentionally need the block-shaped fallback

This follows Notion's newer markdown APIs and usually gives the highest fidelity with the least client-side reconstruction.

### 2. Prefer the smallest precise write

- use `write-section` or `write-page -update-section` when only one stable heading changed
- use full-page writes only when the whole page should be refreshed
- use `-page-id` when you know the exact target page
- use child-page writes when title matching under one parent is the right abstraction

### 3. Let the CLI choose the heavier path only when content requires it

- text-only and remote-media-only content stays on Notion's markdown API
- local images, files, PDFs, audio, and video switch automatically to native Notion uploads plus block-backed writes
- larger local files use multipart uploads automatically
- section updates with local media use a surgical section block swap instead of a full-page rewrite

### 4. Use explicit data-source commands for structured data

- use `list-data-sources`, `get-database`, and `get-data-source` to discover live structure
- use `query-data-source` for filtering, sorting, pagination, or lightweight search
- use `create-row` and `update-row-properties` for row properties instead of pushing pseudo-table markdown
- use `read-page`, `write-page`, and `write-section` on a row body after you know the row page id

### 5. Use native block commands when the content is truly block-shaped

- use `get-block` and `list-block-children` to inspect exact native structure without flattening the page into markdown first
- use `append-block-children`, `update-block`, and `delete-block` when you need native embeds, bookmarks, columns, breadcrumbs, table-of-contents markers, or other precise block edits
- use recursive `list-block-children` when you want a readable markdown-like view of nested columns or other block trees
- prefer these block commands over broader page rewrites when the task is clearly about exact block placement or preservation

## Current model

- configure writes the provided token plus one shared root page id into `.envrc` by default
- reads default to Notion's markdown endpoint for page content
- `read-page` gives a direct page-id based read without requiring a root-tree traversal
- `-read-mode blocks` stays an explicit alternative view instead of silent fallback behavior
- markdown reads automatically try to recover `unknown_block_ids` by fetching those markdown fragments and stitching them back into the page output
- markdown reads can include meeting-note transcript text when `-include-transcript` is set
- if the markdown response is still incomplete after recovery, treat it as partial and choose the next step intentionally
- by default, one markdown file updates the configured write root page directly
- direct root-page writes preserve the existing Notion page title unless `-update-page-title` is set
- child pages are matched by title under the chosen parent
- if a matching child exists, it is updated in place
- if it does not exist, a text-only or remote-media-only page is created directly from markdown
- pages with local media use native Notion uploads and block-backed writes
- block commands expose exact native block inspection and mutation without needing a page rewrite
- `search` supports both workspace search and root-scoped discovery
- full-page writes with local media use append-then-archive block sync
- section updates with local media replace only the matching section boundary instead of rewriting the whole page
- data-source commands expose discovery, querying, row creation, row property updates, and schema updates directly
- row page ids work with `read-page`, `write-page`, and `write-section`
- `get-row-property` now supports `-page-size` and `-cursor` for paginated property-item reads

## Current markdown and block-backed support

Block-backed writes and block-mode reads currently support:

- headings 1-3
- paragraphs
- bulleted lists
- numbered lists
- to-dos
- block quotes
- fenced code blocks
- callouts
- dividers
- equations
- toggles with child blocks
- tables
- page references
- images
- `<file src="...">caption</file>`
- `<pdf src="...">caption</pdf>`
- `<audio src="...">caption</audio>`
- `<video src="...">caption</video>`
- inline code
- bold
- italic
- strikethrough
- underline
- links
- page mentions
- user mentions
- date mentions
- template mentions
- link preview mentions
- inline colors
- native enhanced markdown forms such as `<callout>`, `<table>`, `<details>`, `<mention-page>`, `<mention-user>`, `<mention-date>`, `<page>`, and `<span color="...">`
- block-mode reads also render columns, synced blocks, embeds, bookmarks, breadcrumbs, templates, link previews, and table-of-contents markers

## High-fidelity guidance

- prefer markdown-only writes for the highest fidelity when the source does not need local uploads
- prefer `write-section` for small, stable updates instead of full-page replacement
- use row property commands for structured fields and page commands for the row body
- use `query-data-source` with raw JSON filters and sorts when you need exact Notion query semantics
- keep page titles stable unless the task explicitly changes them
- write user-facing URLs as clickable links, for example `[repo](https://example.com)`, instead of plain text or inline code unless there is a strong reason not to
- use `-dry-run` before uncertain writes
- treat file-backed media URLs returned by reads as temporary signed URLs
- prefer block-native commands over markdown rewrites when the task is about exact block placement or preservation

## Known limits

- database references and database mentions inside markdown are still not trustworthy enough to advertise as stable round-trips
- if a markdown read remains incomplete after recovery, the result is still partial
- some unsupported or inaccessible Notion block types may require `-read-mode blocks` to inspect safely
- synced pages or missing `read content` or `update content` capabilities can still block writes
- synced block creation is still constrained by the Notion API, which may require an existing `synced_from` reference instead of letting you create a new synced source block directly
- comment commands exist in the CLI but should not be part of the default skill workflow for now because many integrations return `403 restricted_resource` on the comments endpoints
