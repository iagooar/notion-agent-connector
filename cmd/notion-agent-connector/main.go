package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"notion-agent-connector/internal/config"
	"notion-agent-connector/internal/notion"
)

type writePlan struct {
	Scope             string
	Strategy          string
	LocalUploadCount  int
	LocalUploadByKind []string
}

func main() {
	if len(os.Args) < 2 {
		exitf("usage: notion-agent-connector <configure|capabilities|get-block|list-block-children|append-block-children|update-block|delete-block|search|list-comments|create-comment|get-database|get-data-source|list-data-sources|query-data-source|create-row|get-row|get-row-property|update-row-properties|update-data-source|read-page|read-root|write-page|write-section> [flags]")
	}

	switch os.Args[1] {
	case "configure":
		if err := runConfigure(os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "capabilities":
		runCapabilities()
		return
	case "get-block":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runGetBlock(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "list-block-children":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runListBlockChildren(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "append-block-children":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runAppendBlockChildren(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "update-block":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runUpdateBlock(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "delete-block":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runDeleteBlock(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "search":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runSearch(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "list-comments":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runListComments(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "create-comment":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runCreateComment(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "get-database":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runGetDatabase(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "list-data-sources":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runListDataSources(context.Background(), client, cfg, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "get-data-source":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runGetDataSource(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "query-data-source":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runQueryDataSource(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "create-row":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runCreateRow(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "get-row":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runGetRow(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "get-row-property":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runGetRowProperty(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "update-row-properties":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runUpdateRowProperties(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "update-data-source":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runUpdateDataSource(context.Background(), client, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "write-page":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runWritePage(context.Background(), client, cfg, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "write-section":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runWriteSection(context.Background(), client, cfg, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "read-page":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runReadPage(context.Background(), client, cfg, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	case "read-root":
		cfg := config.Load()
		client := notion.NewClient(cfg.NotionAccessToken, nil, cfg.NotionBaseURL)
		if err := runReadRoot(context.Background(), client, cfg, os.Args[2:]); err != nil {
			exitf("%v", err)
		}
	default:
		exitf("unknown command %q", os.Args[1])
	}
}

func runWritePage(ctx context.Context, client *notion.Client, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("write-page", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		source            string
		title             string
		pageID            string
		parentPageID      string
		asChildPage       bool
		updateTitle       bool
		updateSection     string
		matchHeadingLevel int
		dryRun            bool
	)
	fs.StringVar(&source, "source", "", "path to the markdown source file")
	fs.StringVar(&title, "title", "", "title for the Notion child page")
	fs.StringVar(&pageID, "page-id", "", "write directly to a specific page id")
	fs.StringVar(&parentPageID, "parent-page-id", "", "parent page id to use when -as-child-page is set")
	fs.BoolVar(&asChildPage, "as-child-page", false, "create or update a child page under the configured write root instead of writing directly to the root page")
	fs.BoolVar(&updateTitle, "update-page-title", false, "when writing directly to the root page, also update the Notion page title")
	fs.StringVar(&updateSection, "update-section", "", "replace only the named heading section instead of refreshing the whole page")
	fs.IntVar(&matchHeadingLevel, "match-heading-level", 0, "optional heading level to disambiguate -update-section matches")
	fs.BoolVar(&dryRun, "dry-run", false, "validate and summarize without writing to Notion")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(cfg.NotionAccessToken) == "" {
		return errors.New("NOTION_AGENT_CONNECTOR_ACCESS_TOKEN is required")
	}
	rootPageID := strings.TrimSpace(cfg.WriteRootPageID)
	if rootPageID == "" {
		return errors.New("NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID is required")
	}
	if strings.TrimSpace(source) == "" {
		return errors.New("-source is required")
	}

	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	doc, err := notion.ParseMarkdown(string(raw))
	if err != nil {
		return err
	}
	if strings.TrimSpace(title) != "" {
		doc.Title = strings.TrimSpace(title)
	}
	notion.ResolveRelativeAssetPaths(&doc, filepath.Dir(source))
	if err := validateLocalAssets(doc, source); err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("Ready to publish\n")
		fmt.Printf("Source: %s\n", source)
		fmt.Printf("Title: %s\n", doc.Title)
		plan := buildWritePlan(doc, asChildPage, strings.TrimSpace(updateSection) != "")
		if strings.TrimSpace(updateSection) != "" {
			fmt.Printf("Section mode: %s\n", strings.TrimSpace(updateSection))
			if matchHeadingLevel > 0 {
				fmt.Printf("Heading level: %d\n", matchHeadingLevel)
			}
		}
		if asChildPage {
			parentPageID = firstNonEmpty(parentPageID, rootPageID)
			fmt.Printf("Mode: child page\n")
			fmt.Printf("Parent page: %s\n", parentPageID)
		} else if strings.TrimSpace(pageID) != "" {
			fmt.Printf("Mode: direct page\n")
			fmt.Printf("Target page: %s\n", strings.TrimSpace(pageID))
		} else {
			fmt.Printf("Mode: root page\n")
			fmt.Printf("Target page: %s\n", rootPageID)
			fmt.Printf("Update title: %t\n", updateTitle)
		}
		fmt.Printf("Write path: %s\n", plan.Strategy)
		if plan.LocalUploadCount > 0 {
			fmt.Printf("Local uploads: %d\n", plan.LocalUploadCount)
			fmt.Printf("Local upload kinds: %s\n", strings.Join(plan.LocalUploadByKind, ", "))
		}
		fmt.Printf("Blocks: %d\n", len(doc.Blocks))
		return nil
	}

	plan := buildWritePlan(doc, asChildPage, strings.TrimSpace(updateSection) != "")
	fmt.Fprintf(os.Stderr, "Write path: %s\n", plan.Strategy)
	if plan.LocalUploadCount > 0 {
		fmt.Fprintf(os.Stderr, "Local uploads: %d (%s)\n", plan.LocalUploadCount, strings.Join(plan.LocalUploadByKind, ", "))
	}
	fmt.Fprintf(os.Stderr, "Publishing to Notion...\n")

	var (
		result   notion.UpsertResult
		writeErr error
	)
	if strings.TrimSpace(pageID) != "" {
		if strings.TrimSpace(updateSection) != "" {
			result, writeErr = client.UpdateDocumentSection(ctx, strings.TrimSpace(pageID), doc, updateSection, matchHeadingLevel, updateTitle)
		} else {
			result, writeErr = client.UpdateDocument(ctx, strings.TrimSpace(pageID), doc, false, updateTitle)
		}
	} else if asChildPage {
		parentPageID = firstNonEmpty(parentPageID, rootPageID)
		if strings.TrimSpace(parentPageID) == "" {
			return errors.New("NOTION_AGENT_CONNECTOR_WRITE_ROOT_PAGE_ID or -parent-page-id is required")
		}
		if strings.TrimSpace(updateSection) != "" {
			targetPageID, found, err := client.FindChildPage(ctx, parentPageID, doc.Title)
			if err != nil {
				return err
			}
			if !found {
				result, writeErr = client.CreateChildDocument(ctx, parentPageID, doc)
			} else {
				result, writeErr = client.UpdateDocumentSection(ctx, targetPageID, doc, updateSection, matchHeadingLevel, true)
			}
		} else {
			result, writeErr = client.UpsertDocument(ctx, parentPageID, doc)
		}
	} else {
		if strings.TrimSpace(updateSection) != "" {
			result, writeErr = client.UpdateDocumentSection(ctx, rootPageID, doc, updateSection, matchHeadingLevel, updateTitle)
		} else {
			result, writeErr = client.UpdateDocument(ctx, rootPageID, doc, false, updateTitle)
		}
	}
	if writeErr != nil {
		return writeErr
	}
	fmt.Fprintf(os.Stderr, "Publish finished\n")
	fmt.Printf("Published %s\n", result.PageURL)
	fmt.Printf("Created: %t\n", result.Created)
	fmt.Printf("Blocks: %d\n", result.BlockCount)
	return nil
}

func runWriteSection(ctx context.Context, client *notion.Client, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("write-section", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		source            string
		title             string
		pageID            string
		parentPageID      string
		heading           string
		matchHeadingLevel int
		asChildPage       bool
		updateTitle       bool
		dryRun            bool
	)
	fs.StringVar(&source, "source", "", "path to the markdown source file")
	fs.StringVar(&title, "title", "", "title for the Notion child page")
	fs.StringVar(&pageID, "page-id", "", "write directly to a specific page id")
	fs.StringVar(&parentPageID, "parent-page-id", "", "parent page id to use when -as-child-page is set")
	fs.StringVar(&heading, "heading", "", "heading to replace")
	fs.IntVar(&matchHeadingLevel, "match-heading-level", 0, "optional heading level to disambiguate -heading matches")
	fs.BoolVar(&asChildPage, "as-child-page", false, "target a child page under the configured write root")
	fs.BoolVar(&updateTitle, "update-page-title", false, "also update the page title when writing directly to a page")
	fs.BoolVar(&dryRun, "dry-run", false, "validate and summarize without writing to Notion")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(heading) == "" {
		return errors.New("-heading is required")
	}

	repacked := []string{
		"-source", source,
		"-update-section", heading,
		"-title", title,
		"-page-id", pageID,
		"-parent-page-id", parentPageID,
	}
	if matchHeadingLevel > 0 {
		repacked = append(repacked, "-match-heading-level", fmt.Sprintf("%d", matchHeadingLevel))
	}
	if asChildPage {
		repacked = append(repacked, "-as-child-page")
	}
	if updateTitle {
		repacked = append(repacked, "-update-page-title")
	}
	if dryRun {
		repacked = append(repacked, "-dry-run")
	}
	return runWritePage(ctx, client, cfg, repacked)
}

func runReadPage(ctx context.Context, client *notion.Client, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("read-page", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		pageID            string
		format            string
		maxDepth          int
		readMode          string
		includeTranscript bool
	)
	fs.StringVar(&pageID, "page-id", "", "page id to read")
	fs.StringVar(&format, "format", "markdown", "output format: markdown or json")
	fs.IntVar(&maxDepth, "max-depth", 0, "maximum child page recursion depth")
	fs.StringVar(&readMode, "read-mode", "markdown", "read mode: markdown or blocks")
	fs.BoolVar(&includeTranscript, "include-transcript", false, "include meeting note transcripts in markdown reads")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(pageID) == "" {
		return errors.New("-page-id is required")
	}
	return renderReadPage(ctx, client, strings.TrimSpace(pageID), format, maxDepth, readMode, includeTranscript)
}

func runReadRoot(ctx context.Context, client *notion.Client, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("read-root", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		rootPageID        string
		format            string
		maxDepth          int
		readMode          string
		includeTranscript bool
	)
	fs.StringVar(&rootPageID, "root-page-id", "", "override the configured read root page id")
	fs.StringVar(&format, "format", "markdown", "output format: markdown or json")
	fs.IntVar(&maxDepth, "max-depth", 4, "maximum child page recursion depth")
	fs.StringVar(&readMode, "read-mode", "markdown", "read mode: markdown or blocks")
	fs.BoolVar(&includeTranscript, "include-transcript", false, "include meeting note transcripts in markdown reads")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(cfg.NotionAccessToken) == "" {
		return errors.New("NOTION_AGENT_CONNECTOR_ACCESS_TOKEN is required")
	}
	rootPageID = firstNonEmpty(rootPageID, cfg.ReadRootPageID)
	if strings.TrimSpace(rootPageID) == "" {
		return errors.New("NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID or -root-page-id is required")
	}
	if maxDepth < 0 {
		return errors.New("-max-depth must be >= 0")
	}

	return renderReadPage(ctx, client, rootPageID, format, maxDepth, readMode, includeTranscript)
}

func renderReadPage(ctx context.Context, client *notion.Client, pageID string, format string, maxDepth int, readMode string, includeTranscript bool) error {
	tree, err := client.ReadPageTreeWithOptions(ctx, pageID, maxDepth, notion.ReadOptions{
		Mode:              notion.ReadMode(readMode),
		IncludeTranscript: includeTranscript,
	})
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "markdown", "md":
		fmt.Print(tree.RenderMarkdown())
		return nil
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(tree)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func runCapabilities() {
	fmt.Println("Notion Agent Connector CLI capabilities")
	fmt.Println("")
	fmt.Println("Read commands:")
	fmt.Println("- get-block: inspect one native block")
	fmt.Println("- list-block-children: inspect direct children or descendants of a block")
	fmt.Println("- search: search the workspace or a root-scoped page tree")
	fmt.Println("- list-comments: inspect comments on a page or block")
	fmt.Println("- read-root: read the configured Notion root page tree")
	fmt.Println("- read-page: read one specific page by id")
	fmt.Println("- list-data-sources: discover child databases and data sources")
	fmt.Println("- get-database: inspect one database and its data sources")
	fmt.Println("- get-data-source: inspect one data source schema")
	fmt.Println("- query-data-source: query rows with filters, sorts, and pagination")
	fmt.Println("- get-row: inspect one row page and its properties")
	fmt.Println("- get-row-property: inspect one row property item")
	fmt.Println("")
	fmt.Println("Write commands:")
	fmt.Println("- append-block-children: append native child blocks with raw block JSON")
	fmt.Println("- update-block: patch one native block with raw block JSON")
	fmt.Println("- delete-block: archive one native block")
	fmt.Println("- create-comment: create a page or block comment, or reply to a discussion")
	fmt.Println("- write-page: create or replace a full page")
	fmt.Println("- write-section: replace one heading section surgically")
	fmt.Println("- create-row: create one page row under a data source")
	fmt.Println("- update-row-properties: update row properties through the page API")
	fmt.Println("- update-data-source: update data source title, description, or schema")
	fmt.Println("")
	fmt.Println("Targeting:")
	fmt.Println("- -page-id writes directly to one page")
	fmt.Println("- -as-child-page with -title targets a child page under the configured write root")
	fmt.Println("- without -page-id or -as-child-page, writes target the configured root page")
	fmt.Println("- row page ids work with read-page, write-page, and write-section")
	fmt.Println("")
	fmt.Println("Write strategies:")
	fmt.Println("- markdown replace_content for full-page markdown updates")
	fmt.Println("- markdown update_content for markdown-only section updates")
	fmt.Println("- surgical section block swap for section updates that include local media")
	fmt.Println("- native block sync with append-then-archive for full-page writes that include local media")
	fmt.Println("")
	fmt.Println("Database and data source support:")
	fmt.Println("- create rows with the same Notion-shaped properties payload used by the page API")
	fmt.Println("- query data sources with raw JSON filters and sorts, or use -title-contains as a shorthand")
	fmt.Println("- update row properties and retrieve individual property items with pagination")
	fmt.Println("- update data source title, description, and schema properties")
	fmt.Println("")
	fmt.Println("Block-backed writes currently support:")
	fmt.Println("- paragraphs, headings 1-3, bullets, numbered lists, to-dos, quotes, callouts, dividers, code, equations, toggles with child blocks, tables, page references, images, files, PDFs, audio, video")
	fmt.Println("- inline code, bold, italic, strikethrough, underline, links, page mentions, user mentions, date mentions, template mentions, link preview mentions, and inline colors")
	fmt.Println("- block reads also render columns, synced blocks, embeds, bookmarks, breadcrumbs, link previews, templates, and table of contents markers")
	fmt.Println("- native enhanced markdown forms such as <callout>, <table>, <details>, <mention-page>, <mention-user>, <mention-date>, <page>, and <span color=\"...\">")
	fmt.Println("")
	fmt.Println("Database references and database mentions are still not trustworthy enough to advertise as stable markdown round-trips.")
	fmt.Println("When possible, prefer markdown-only writes for the highest fidelity.")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func buildWritePlan(doc notion.Document, asChildPage bool, updateSection bool) writePlan {
	plan := writePlan{
		Scope:    "root page",
		Strategy: "markdown replace_content",
	}
	if asChildPage {
		plan.Scope = "child page"
	}
	if updateSection {
		if asChildPage {
			plan.Strategy = "markdown update_content for an existing child page, or markdown create for a missing child page"
		} else {
			plan.Strategy = "markdown update_content"
		}
	}
	localUploadsByKind := localUploadsByKind(doc)
	plan.LocalUploadByKind = localUploadsByKind
	for _, item := range localUploadsByKind {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			plan.LocalUploadCount += atoiDefault(parts[1])
		}
	}
	if plan.LocalUploadCount > 0 {
		if updateSection {
			plan.Strategy = "surgical section block swap with local media uploads"
		} else {
			plan.Strategy = "native block sync with append-then-archive because local media uploads are present"
		}
	}
	return plan
}

func validateLocalAssets(doc notion.Document, source string) error {
	sourceDir := filepath.Dir(source)
	for _, block := range doc.Blocks {
		if err := validateLocalAssetsInBlock(block, source, sourceDir); err != nil {
			return err
		}
	}
	return nil
}

func validateLocalAssetsInBlock(block notion.Block, source string, sourceDir string) error {
	if block.AssetPath != "" && !isRemoteAssetPath(block.AssetPath) {
		if _, err := os.Stat(block.AssetPath); err != nil {
			return fmt.Errorf("local %s asset %q could not be read from source %s (relative paths are resolved from %s): %w", block.Kind, block.AssetPath, source, sourceDir, err)
		}
	}
	for _, child := range block.Children {
		if err := validateLocalAssetsInBlock(child, source, sourceDir); err != nil {
			return err
		}
	}
	return nil
}

func localUploadsByKind(doc notion.Document) []string {
	counts := map[string]int{}
	order := []string{"image", "pdf", "file", "audio", "video"}
	for _, block := range doc.Blocks {
		accumulateLocalUploads(counts, block)
	}
	items := make([]string, 0, len(counts))
	for _, kind := range order {
		if counts[kind] == 0 {
			continue
		}
		items = append(items, fmt.Sprintf("%s=%d", kind, counts[kind]))
	}
	return items
}

func accumulateLocalUploads(counts map[string]int, block notion.Block) {
	if block.AssetPath != "" && !isRemoteAssetPath(block.AssetPath) {
		counts[block.Kind]++
	}
	for _, child := range block.Children {
		accumulateLocalUploads(counts, child)
	}
}

func isRemoteAssetPath(path string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(path))
	return strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
}

func atoiDefault(value string) int {
	var parsed int
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	return parsed
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
