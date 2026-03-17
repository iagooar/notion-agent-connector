package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"notion-agent-connector/internal/notion"
)

func runGetBlock(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("get-block", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		blockID string
		format  string
	)
	fs.StringVar(&blockID, "block-id", "", "block id to retrieve")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(blockID) == "" {
		return errors.New("-block-id is required")
	}
	block, err := client.RetrieveBlockRaw(ctx, strings.TrimSpace(blockID))
	if err != nil {
		return err
	}
	return renderBlockValue(block, format)
}

func runListBlockChildren(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("list-block-children", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		blockID   string
		cursor    string
		format    string
		pageSize  int
		recursive bool
	)
	fs.StringVar(&blockID, "block-id", "", "block id whose children should be listed")
	fs.StringVar(&cursor, "cursor", "", "optional pagination cursor")
	fs.IntVar(&pageSize, "page-size", 0, "optional page size")
	fs.BoolVar(&recursive, "recursive", false, "list descendants recursively instead of one page of direct children")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(blockID) == "" {
		return errors.New("-block-id is required")
	}
	if recursive {
		blocks, err := client.ListBlockChildrenRecursiveRaw(ctx, strings.TrimSpace(blockID))
		if err != nil {
			return err
		}
		page := notion.BlockChildrenPage{Results: blocks}
		return renderBlockChildrenPage(page, format)
	}
	page, err := client.ListBlockChildrenPageRaw(ctx, strings.TrimSpace(blockID), strings.TrimSpace(cursor), pageSize)
	if err != nil {
		return err
	}
	return renderBlockChildrenPage(page, format)
}

func runAppendBlockChildren(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("append-block-children", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		blockID      string
		childrenJSON string
		afterBlockID string
		prepend      bool
		format       string
	)
	fs.StringVar(&blockID, "block-id", "", "parent block id to append children to")
	fs.StringVar(&childrenJSON, "children-json", "", "JSON array or @path to child block payloads")
	fs.StringVar(&afterBlockID, "after-block-id", "", "append after a specific child block id")
	fs.BoolVar(&prepend, "prepend", false, "insert at the start instead of appending to the end")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(blockID) == "" {
		return errors.New("-block-id is required")
	}
	children, err := loadJSONArrayValue(childrenJSON)
	if err != nil {
		return err
	}
	payloads := make([]map[string]any, 0, len(children))
	for _, item := range children {
		child, ok := item.(map[string]any)
		if !ok {
			return errors.New("children JSON must contain block objects")
		}
		payloads = append(payloads, child)
	}

	position := notionAppendPosition(afterBlockID, prepend)
	page, err := client.AppendBlockChildrenRaw(ctx, strings.TrimSpace(blockID), payloads, position)
	if err != nil {
		return err
	}
	return renderBlockChildrenPage(page, format)
}

func runUpdateBlock(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("update-block", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		blockID   string
		blockJSON string
		format    string
	)
	fs.StringVar(&blockID, "block-id", "", "block id to update")
	fs.StringVar(&blockJSON, "block-json", "", "JSON object or @path to a block update payload")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(blockID) == "" {
		return errors.New("-block-id is required")
	}
	payload, err := loadJSONObjectValue(blockJSON)
	if err != nil {
		return err
	}
	block, err := client.UpdateBlockRaw(ctx, strings.TrimSpace(blockID), payload)
	if err != nil {
		return err
	}
	return renderBlockValue(block, format)
}

func runDeleteBlock(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("delete-block", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		blockID string
		format  string
	)
	fs.StringVar(&blockID, "block-id", "", "block id to archive")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(blockID) == "" {
		return errors.New("-block-id is required")
	}
	block, err := client.DeleteBlockRaw(ctx, strings.TrimSpace(blockID))
	if err != nil {
		return err
	}
	return renderBlockValue(block, format)
}

func runSearch(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		query            string
		filterJSON       string
		sortJSON         string
		cursor           string
		format           string
		rootPageID       string
		pageSize         int
		maxDepth         int
		includeRowTitles bool
	)
	fs.StringVar(&query, "query", "", "workspace or root-scoped search query")
	fs.StringVar(&filterJSON, "filter-json", "", "optional raw search filter JSON or @path")
	fs.StringVar(&sortJSON, "sort-json", "", "optional raw search sort JSON or @path")
	fs.StringVar(&cursor, "cursor", "", "optional search cursor")
	fs.IntVar(&pageSize, "page-size", 0, "optional page size")
	fs.StringVar(&rootPageID, "root-page-id", "", "optional root page id for root-scoped discovery instead of workspace search")
	fs.IntVar(&maxDepth, "max-depth", 3, "maximum recursion depth for root-scoped search")
	fs.BoolVar(&includeRowTitles, "include-row-titles", false, "also search titles in discovered data-source rows when using -root-page-id")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(rootPageID) != "" {
		result, err := client.SearchRoot(ctx, strings.TrimSpace(rootPageID), notion.RootSearchOptions{
			Query:            strings.TrimSpace(query),
			MaxDepth:         maxDepth,
			IncludeRowTitles: includeRowTitles,
		})
		if err != nil {
			return err
		}
		return renderSearchSummary(result, format)
	}

	filter, err := maybeLoadJSONValue(filterJSON)
	if err != nil {
		return err
	}
	sortValue, err := maybeLoadJSONValue(sortJSON)
	if err != nil {
		return err
	}
	result, err := client.SearchWorkspace(ctx, notion.SearchOptions{
		Query:       strings.TrimSpace(query),
		Filter:      filter,
		Sort:        sortValue,
		StartCursor: strings.TrimSpace(cursor),
		PageSize:    pageSize,
	})
	if err != nil {
		return err
	}
	return renderSearchSummary(notion.SummarizeSearchResult("workspace", strings.TrimSpace(query), result), format)
}

func runListComments(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("list-comments", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		blockID  string
		pageID   string
		cursor   string
		format   string
		pageSize int
	)
	fs.StringVar(&blockID, "block-id", "", "page or block id whose comment thread should be listed")
	fs.StringVar(&pageID, "page-id", "", "page id whose comments should be listed")
	fs.StringVar(&cursor, "cursor", "", "optional comment pagination cursor")
	fs.IntVar(&pageSize, "page-size", 0, "optional comment page size")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	targetID := firstNonEmpty(pageID, blockID)
	if strings.TrimSpace(targetID) == "" {
		return errors.New("-page-id or -block-id is required")
	}
	result, err := client.ListComments(ctx, strings.TrimSpace(targetID), strings.TrimSpace(cursor), pageSize)
	if err != nil {
		return err
	}
	return renderComments(notion.SummarizeCommentList(result), format)
}

func runCreateComment(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("create-comment", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		pageID       string
		blockID      string
		discussionID string
		text         string
		richTextJSON string
		format       string
	)
	fs.StringVar(&pageID, "page-id", "", "page id to comment on")
	fs.StringVar(&blockID, "block-id", "", "block id to comment on")
	fs.StringVar(&discussionID, "discussion-id", "", "existing discussion id to reply to")
	fs.StringVar(&text, "text", "", "plain text or inline markdown-style comment text")
	fs.StringVar(&richTextJSON, "rich-text-json", "", "optional rich_text JSON array or @path")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(discussionID) == "" && strings.TrimSpace(pageID) == "" && strings.TrimSpace(blockID) == "" {
		return errors.New("one of -page-id, -block-id, or -discussion-id is required")
	}

	var richText []any
	if strings.TrimSpace(richTextJSON) != "" {
		loaded, err := loadJSONArrayValue(richTextJSON)
		if err != nil {
			return err
		}
		richText = loaded
	} else if strings.TrimSpace(text) != "" {
		richText = notion.RichTextInputFromText(strings.TrimSpace(text))
	} else {
		return errors.New("-text or -rich-text-json is required")
	}

	comment, err := client.CreateComment(ctx, notion.CommentCreateInput{
		PageID:       strings.TrimSpace(pageID),
		BlockID:      strings.TrimSpace(blockID),
		DiscussionID: strings.TrimSpace(discussionID),
		RichText:     richText,
	})
	if err != nil {
		return err
	}
	return renderComment(notion.SummarizeComment(comment), format)
}

func renderBlockValue(block map[string]any, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(block)
	case "markdown", "md":
		fmt.Printf("# Block\n\n")
		fmt.Printf("- ID: %s\n", strings.TrimSpace(fmt.Sprintf("%v", block["id"])))
		fmt.Printf("- Type: %s\n", strings.TrimSpace(fmt.Sprintf("%v", block["type"])))
		fmt.Printf("- Has children: %t\n", block["has_children"] == true)
		markdown := notion.RenderRawBlockMarkdown(block)
		if strings.TrimSpace(markdown) != "" {
			fmt.Printf("\n## Content\n\n%s\n", markdown)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderBlockChildrenPage(page notion.BlockChildrenPage, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(page)
	case "markdown", "md":
		fmt.Printf("# Block Children\n\n")
		fmt.Printf("- Count: %d\n", len(page.Results))
		fmt.Printf("- Has more: %t\n", page.HasMore)
		if page.NextCursor != "" {
			fmt.Printf("- Next cursor: %s\n", page.NextCursor)
		}
		if markdown := notion.RenderRawBlocksMarkdown(page.Results); strings.TrimSpace(markdown) != "" {
			fmt.Printf("\n## Content\n\n%s\n", markdown)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderSearchSummary(summary notion.SearchResultSummary, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(summary)
	case "markdown", "md":
		fmt.Printf("# Search\n\n")
		fmt.Printf("- Scope: %s\n", summary.Scope)
		if summary.Query != "" {
			fmt.Printf("- Query: %s\n", summary.Query)
		}
		fmt.Printf("- Entries: %d\n", len(summary.Entries))
		fmt.Printf("- Has more: %t\n", summary.HasMore)
		if summary.NextCursor != "" {
			fmt.Printf("- Next cursor: %s\n", summary.NextCursor)
		}
		fmt.Println("")
		for _, entry := range summary.Entries {
			fmt.Printf("## %s\n\n", firstNonEmpty(entry.Title, entry.ID))
			fmt.Printf("- Object: %s\n", entry.Object)
			fmt.Printf("- ID: %s\n", entry.ID)
			if entry.URL != "" {
				fmt.Printf("- URL: %s\n", entry.URL)
			}
			fmt.Println("")
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderComments(summary notion.CommentSummaryList, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(summary)
	case "markdown", "md":
		fmt.Printf("# Comments\n\n")
		fmt.Printf("- Count: %d\n", len(summary.Comments))
		fmt.Printf("- Has more: %t\n", summary.HasMore)
		if summary.NextCursor != "" {
			fmt.Printf("- Next cursor: %s\n", summary.NextCursor)
		}
		fmt.Println("")
		for _, comment := range summary.Comments {
			fmt.Printf("## %s\n\n", firstNonEmpty(comment.Text, comment.ID))
			fmt.Printf("- ID: %s\n", comment.ID)
			if comment.DiscussionID != "" {
				fmt.Printf("- Discussion ID: %s\n", comment.DiscussionID)
			}
			if comment.ParentType != "" {
				fmt.Printf("- Parent %s: %s\n", comment.ParentType, comment.ParentID)
			}
			if comment.CreatedBy != "" {
				fmt.Printf("- Created by: %s\n", comment.CreatedBy)
			}
			if comment.CreatedTime != "" {
				fmt.Printf("- Created time: %s\n", comment.CreatedTime)
			}
			fmt.Printf("- Text: %s\n\n", comment.Text)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderComment(summary notion.CommentSummary, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(summary)
	case "markdown", "md":
		fmt.Printf("# Comment\n\n")
		fmt.Printf("- ID: %s\n", summary.ID)
		if summary.DiscussionID != "" {
			fmt.Printf("- Discussion ID: %s\n", summary.DiscussionID)
		}
		if summary.ParentType != "" {
			fmt.Printf("- Parent %s: %s\n", summary.ParentType, summary.ParentID)
		}
		if summary.CreatedBy != "" {
			fmt.Printf("- Created by: %s\n", summary.CreatedBy)
		}
		if summary.CreatedTime != "" {
			fmt.Printf("- Created time: %s\n", summary.CreatedTime)
		}
		fmt.Printf("- Text: %s\n", summary.Text)
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func maybeLoadJSONValue(value string) (any, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	return loadJSONValue(value)
}

func notionAppendPosition(afterBlockID string, prepend bool) notion.AppendPosition {
	switch {
	case strings.TrimSpace(afterBlockID) != "":
		return notion.AppendPosition{Kind: "after_block", AfterBlockID: strings.TrimSpace(afterBlockID)}
	case prepend:
		return notion.AppendPosition{Kind: "start"}
	default:
		return notion.AppendPosition{}
	}
}
