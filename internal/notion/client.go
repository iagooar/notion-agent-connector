package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL                = "https://api.notion.com/v1"
	notionVersion                 = "2026-03-11"
	maxRequestTries               = 4
	maxSinglePartUploadSize int64 = 20 * 1024 * 1024
	multipartUploadPartSize int64 = 10 * 1024 * 1024
	blockArchiveConcurrency       = 3
	blockArchiveTimeout           = 15 * time.Second
)

var unknownMarkdownTagRE = regexp.MustCompile(`<unknown\b[^>]*/>`)
var markdownLinkTrailingSlashRE = regexp.MustCompile(`\((https?://[^)\s]+?)/\)`)

type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

type UpsertResult struct {
	PageID     string
	PageURL    string
	Created    bool
	BlockCount int
}

type PageMarkdown struct {
	ID              string   `json:"id"`
	Markdown        string   `json:"markdown"`
	Truncated       bool     `json:"truncated"`
	UnknownBlockIDs []string `json:"unknown_block_ids"`
}

type ContentUpdate struct {
	OldStr            string
	NewStr            string
	ReplaceAllMatches bool
}

type ReadMode string

const (
	ReadModeMarkdown ReadMode = "markdown"
	ReadModeBlocks   ReadMode = "blocks"
)

type ReadOptions struct {
	Mode              ReadMode
	IncludeTranscript bool
}

type PageTree struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	URL      string     `json:"url,omitempty"`
	Content  string     `json:"markdown"`
	Children []PageTree `json:"children,omitempty"`
}

type notionBlock struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	HasChildren bool   `json:"has_children,omitempty"`
	Children    []notionBlock
	Paragraph   *struct {
		RichText []richTextItem `json:"rich_text"`
	} `json:"paragraph,omitempty"`
	Heading1 *struct {
		RichText []richTextItem `json:"rich_text"`
	} `json:"heading_1,omitempty"`
	Heading2 *struct {
		RichText []richTextItem `json:"rich_text"`
	} `json:"heading_2,omitempty"`
	Heading3 *struct {
		RichText []richTextItem `json:"rich_text"`
	} `json:"heading_3,omitempty"`
	BulletedListItem *struct {
		RichText []richTextItem `json:"rich_text"`
	} `json:"bulleted_list_item,omitempty"`
	NumberedListItem *struct {
		RichText []richTextItem `json:"rich_text"`
	} `json:"numbered_list_item,omitempty"`
	Toggle *struct {
		RichText []richTextItem `json:"rich_text"`
		Color    string         `json:"color,omitempty"`
	} `json:"toggle,omitempty"`
	ToDo *struct {
		RichText []richTextItem `json:"rich_text"`
		Checked  bool           `json:"checked"`
		Color    string         `json:"color,omitempty"`
	} `json:"to_do,omitempty"`
	Quote *struct {
		RichText []richTextItem `json:"rich_text"`
		Color    string         `json:"color,omitempty"`
	} `json:"quote,omitempty"`
	Divider *struct{} `json:"divider,omitempty"`
	Callout *struct {
		RichText []richTextItem `json:"rich_text"`
		Color    string         `json:"color,omitempty"`
		Icon     *struct {
			Type  string `json:"type"`
			Emoji string `json:"emoji,omitempty"`
		} `json:"icon,omitempty"`
	} `json:"callout,omitempty"`
	Code *struct {
		RichText []richTextItem `json:"rich_text"`
		Language string         `json:"language"`
		Color    string         `json:"color,omitempty"`
	} `json:"code,omitempty"`
	Equation *struct {
		Expression string `json:"expression"`
	} `json:"equation,omitempty"`
	Table *struct {
		TableWidth      int  `json:"table_width"`
		HasColumnHeader bool `json:"has_column_header"`
		HasRowHeader    bool `json:"has_row_header"`
	} `json:"table,omitempty"`
	TableRow *struct {
		Cells [][]richTextItem `json:"cells"`
	} `json:"table_row,omitempty"`
	Template *struct {
		RichText []richTextItem `json:"rich_text"`
	} `json:"template,omitempty"`
	SyncedBlock *struct {
		SyncedFrom *struct {
			Type    string `json:"type"`
			BlockID string `json:"block_id,omitempty"`
		} `json:"synced_from,omitempty"`
	} `json:"synced_block,omitempty"`
	Breadcrumb      *struct{} `json:"breadcrumb,omitempty"`
	TableOfContents *struct {
		Color string `json:"color,omitempty"`
	} `json:"table_of_contents,omitempty"`
	ColumnList *struct{} `json:"column_list,omitempty"`
	Column     *struct{} `json:"column,omitempty"`
	Embed      *struct {
		URL string `json:"url"`
	} `json:"embed,omitempty"`
	Bookmark *struct {
		URL string `json:"url"`
	} `json:"bookmark,omitempty"`
	LinkPreview *struct {
		URL string `json:"url"`
	} `json:"link_preview,omitempty"`
	Image      *notionMediaBlockData `json:"image,omitempty"`
	File       *notionMediaBlockData `json:"file,omitempty"`
	PDF        *notionMediaBlockData `json:"pdf,omitempty"`
	Audio      *notionMediaBlockData `json:"audio,omitempty"`
	Video      *notionMediaBlockData `json:"video,omitempty"`
	LinkToPage *struct {
		Type       string `json:"type"`
		PageID     string `json:"page_id,omitempty"`
		DatabaseID string `json:"database_id,omitempty"`
	} `json:"link_to_page,omitempty"`
	ChildPage *struct {
		Title string `json:"title"`
	} `json:"child_page,omitempty"`
	ChildDatabase *struct {
		Title string `json:"title"`
	} `json:"child_database,omitempty"`
}

type notionMediaBlockData struct {
	Caption  []richTextItem `json:"caption"`
	Type     string         `json:"type"`
	External *struct {
		URL string `json:"url"`
	} `json:"external,omitempty"`
	File *struct {
		URL string `json:"url"`
	} `json:"file,omitempty"`
}

type appendPosition struct {
	Kind         string
	AfterBlockID string
}

type richTextItem struct {
	PlainText string `json:"plain_text"`
	Href      string `json:"href,omitempty"`
	Mention   *struct {
		Type string `json:"type"`
		Page *struct {
			ID string `json:"id"`
		} `json:"page,omitempty"`
		Database *struct {
			ID string `json:"id"`
		} `json:"database,omitempty"`
		User *struct {
			ID string `json:"id"`
		} `json:"user,omitempty"`
		Date *struct {
			Start    string `json:"start"`
			End      string `json:"end,omitempty"`
			TimeZone string `json:"time_zone,omitempty"`
		} `json:"date,omitempty"`
		LinkPreview *struct {
			URL string `json:"url"`
		} `json:"link_preview,omitempty"`
		TemplateMention *struct {
			Type                string `json:"type"`
			TemplateMentionDate string `json:"template_mention_date,omitempty"`
			TemplateMentionUser string `json:"template_mention_user,omitempty"`
		} `json:"template_mention,omitempty"`
	} `json:"mention,omitempty"`
	Annotations *struct {
		Bold          bool   `json:"bold"`
		Italic        bool   `json:"italic"`
		Strikethrough bool   `json:"strikethrough"`
		Underline     bool   `json:"underline"`
		Code          bool   `json:"code"`
		Color         string `json:"color"`
	} `json:"annotations,omitempty"`
}

func NewClient(token string, httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		token:      strings.TrimSpace(token),
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (c *Client) UpsertDocument(ctx context.Context, parentPageID string, doc Document) (UpsertResult, error) {
	pageID, found, err := c.findChildPage(ctx, parentPageID, doc.Title)
	if err != nil {
		return UpsertResult{}, err
	}
	if found {
		return c.UpdateDocument(ctx, pageID, doc, false, true)
	}
	return c.CreateChildDocument(ctx, parentPageID, doc)
}

func (c *Client) UpdateDocument(ctx context.Context, pageID string, doc Document, created bool, updateTitle bool) (UpsertResult, error) {
	targetTitle := doc.Title
	if !updateTitle {
		currentTitle, err := c.pageTitle(ctx, pageID)
		if err != nil {
			return UpsertResult{}, err
		}
		targetTitle = currentTitle
	}

	effectiveMarkdown, err := RewriteMarkdownTitle(doc.Markdown, targetTitle)
	if err != nil {
		return UpsertResult{}, err
	}

	if updateTitle {
		if err := c.updatePageTitle(ctx, pageID, targetTitle); err != nil {
			return UpsertResult{}, err
		}
	}

	switch {
	case doc.HasLocalUploads():
		if err := c.replacePageContentSafely(ctx, pageID, doc.Blocks); err != nil {
			return UpsertResult{}, err
		}
	default:
		pageMarkdown, err := c.replacePageMarkdown(ctx, pageID, effectiveMarkdown)
		if err != nil {
			return UpsertResult{}, err
		}
		if comparableMarkdown(pageMarkdown.Markdown) != comparableMarkdown(effectiveMarkdown) {
			return UpsertResult{}, fmt.Errorf("notion markdown verification failed for %s: %s", pageID, markdownDiffSummary(effectiveMarkdown, pageMarkdown.Markdown))
		}
	}

	return UpsertResult{
		PageID:     pageID,
		PageURL:    notionPageURL(pageID),
		Created:    created,
		BlockCount: len(doc.Blocks),
	}, nil
}

func (c *Client) UpdateDocumentSection(ctx context.Context, pageID string, doc Document, heading string, level int, updateTitle bool) (UpsertResult, error) {
	if doc.HasLocalUploads() {
		section, err := ExtractBlockSection(doc.Blocks, heading, level)
		if err != nil {
			return UpsertResult{}, err
		}
		if err := c.replacePageSectionSafely(ctx, pageID, section); err != nil {
			return UpsertResult{}, err
		}
		return UpsertResult{
			PageID:     pageID,
			PageURL:    notionPageURL(pageID),
			Created:    false,
			BlockCount: len(section.Blocks),
		}, nil
	}

	targetTitle := doc.Title
	if !updateTitle {
		currentTitle, err := c.pageTitle(ctx, pageID)
		if err != nil {
			return UpsertResult{}, err
		}
		targetTitle = currentTitle
	}

	effectiveMarkdown, err := RewriteMarkdownTitle(doc.Markdown, targetTitle)
	if err != nil {
		return UpsertResult{}, err
	}

	current, err := c.GetPageMarkdown(ctx, pageID)
	if err != nil {
		return UpsertResult{}, err
	}

	oldSection, err := ExtractSection(current.Markdown, heading, level)
	if err != nil {
		return UpsertResult{}, err
	}
	newSection, err := ExtractSection(effectiveMarkdown, heading, level)
	if err != nil {
		return UpsertResult{}, err
	}

	updated, err := c.updatePageMarkdownSearchReplace(ctx, pageID, []ContentUpdate{{
		OldStr: oldSection.Body,
		NewStr: newSection.Body,
	}})
	if err != nil {
		return UpsertResult{}, err
	}

	verified, err := ExtractSection(updated.Markdown, heading, level)
	if err != nil {
		return UpsertResult{}, err
	}
	if comparableMarkdown(verified.Body) != comparableMarkdown(newSection.Body) {
		return UpsertResult{}, fmt.Errorf("notion section verification failed for %q", heading)
	}

	return UpsertResult{
		PageID:     pageID,
		PageURL:    notionPageURL(pageID),
		Created:    false,
		BlockCount: len(doc.Blocks),
	}, nil
}

func (c *Client) ReadPageTree(ctx context.Context, rootPageID string, maxDepth int) (PageTree, error) {
	return c.ReadPageTreeWithOptions(ctx, rootPageID, maxDepth, ReadOptions{Mode: ReadModeMarkdown})
}

func (c *Client) ReadPageTreeWithMode(ctx context.Context, rootPageID string, maxDepth int, mode ReadMode) (PageTree, error) {
	return c.ReadPageTreeWithOptions(ctx, rootPageID, maxDepth, ReadOptions{Mode: mode})
}

func (c *Client) ReadPageTreeWithOptions(ctx context.Context, rootPageID string, maxDepth int, options ReadOptions) (PageTree, error) {
	options.Mode = normalizeReadMode(options.Mode)
	return c.readPageTree(ctx, rootPageID, maxDepth, options)
}

func (t PageTree) RenderMarkdown() string {
	return strings.TrimSpace(t.Content) + "\n"
}

func (c *Client) readPageTree(ctx context.Context, pageID string, depth int, options ReadOptions) (PageTree, error) {
	var (
		title   string
		content string
	)

	pageMarkdown, blocks, err := c.readPageContent(ctx, pageID, options)
	if err != nil {
		return PageTree{}, err
	}
	if pageMarkdown != nil {
		title = extractTitleFromMarkdown(pageMarkdown.Markdown)
		content = NormalizeMarkdown(pageMarkdown.Markdown)
	} else {
		title, err = c.pageTitle(ctx, pageID)
		if err != nil {
			return PageTree{}, err
		}
		content = renderBlocksAsMarkdown(title, blocks)
	}

	children := make([]PageTree, 0)
	for _, block := range blocks {
		if block.Type != "child_page" || depth <= 0 || block.ChildPage == nil {
			continue
		}
		childTree, err := c.readPageTree(ctx, block.ID, depth-1, options)
		if err != nil {
			return PageTree{}, err
		}
		children = append(children, childTree)
	}

	return PageTree{
		ID:       pageID,
		Title:    title,
		URL:      notionPageURL(pageID),
		Content:  content,
		Children: children,
	}, nil
}

func (c *Client) readPageContent(ctx context.Context, pageID string, options ReadOptions) (*PageMarkdown, []notionBlock, error) {
	options.Mode = normalizeReadMode(options.Mode)
	switch options.Mode {
	case ReadModeBlocks:
		blocks, err := c.listBlockChildren(ctx, pageID)
		if err != nil {
			return nil, nil, err
		}
		return nil, blocks, nil
	case ReadModeMarkdown:
		pageMarkdown, err := c.getCompletePageMarkdown(ctx, pageID, options.IncludeTranscript, map[string]bool{})
		if err != nil {
			return nil, nil, err
		}
		if pageMarkdown.Truncated || len(pageMarkdown.UnknownBlockIDs) > 0 {
			return nil, nil, fmt.Errorf("notion markdown read for %s is incomplete; use -read-mode blocks explicitly if you want the alternative block-based view", pageID)
		}
		blocks, err := c.listBlockChildren(ctx, pageID)
		if err != nil {
			return nil, nil, err
		}
		return &pageMarkdown, blocks, nil
	default:
		return nil, nil, fmt.Errorf("unsupported read mode %q", options.Mode)
	}
}

func (t PageTree) MarkdownWithoutTitle() string {
	lines := strings.Split(t.Content, "\n")
	if len(lines) <= 2 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[2:], "\n"))
}

func extractTitleFromMarkdown(markdown string) string {
	for _, line := range strings.Split(NormalizeMarkdown(markdown), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return "Untitled"
}

func markdownWithoutTitle(markdown string) string {
	lines := strings.Split(NormalizeMarkdown(markdown), "\n")
	if len(lines) == 0 {
		return ""
	}
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[start]), "# ") {
		start++
		for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
			start++
		}
	}
	return strings.Join(lines[start:], "\n")
}

func isEmptyPageMarkdown(markdown string) bool {
	lines := strings.Split(NormalizeMarkdown(markdown), "\n")
	if len(lines) == 0 {
		return true
	}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i == 0 && strings.HasPrefix(trimmed, "# ") {
			continue
		}
		if trimmed != "" {
			return false
		}
	}
	return true
}

func comparableMarkdown(markdown string) string {
	lines := strings.Split(NormalizeMarkdown(markdown), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func normalizeReadMode(mode ReadMode) ReadMode {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case "", "markdown", "md":
		return ReadModeMarkdown
	case "blocks", "block":
		return ReadModeBlocks
	default:
		return mode
	}
}

func (c *Client) findChildPage(ctx context.Context, parentPageID string, title string) (string, bool, error) {
	children, err := c.listBlockChildren(ctx, parentPageID)
	if err != nil {
		return "", false, err
	}
	for _, child := range children {
		if child.Type == "child_page" && child.ChildPage != nil && strings.TrimSpace(child.ChildPage.Title) == strings.TrimSpace(title) {
			return child.ID, true, nil
		}
	}
	return "", false, nil
}

func (c *Client) createChildPage(ctx context.Context, parentPageID string, title string) (string, error) {
	payload := map[string]any{
		"parent": map[string]any{"type": "page_id", "page_id": parentPageID},
		"properties": map[string]any{
			"title": map[string]any{
				"title": []any{richTextObjectFromInline(Inline{Text: title})},
			},
		},
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.request(ctx, http.MethodPost, "/pages", payload, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.ID) == "" {
		return "", fmt.Errorf("notion create page returned no id")
	}
	return resp.ID, nil
}

func (c *Client) FindChildPage(ctx context.Context, parentPageID string, title string) (string, bool, error) {
	return c.findChildPage(ctx, parentPageID, title)
}

func (c *Client) CreateChildDocument(ctx context.Context, parentPageID string, doc Document) (UpsertResult, error) {
	effectiveMarkdown, err := RewriteMarkdownTitle(doc.Markdown, doc.Title)
	if err != nil {
		return UpsertResult{}, err
	}
	if doc.HasLocalUploads() {
		pageID, err := c.createChildPage(ctx, parentPageID, doc.Title)
		if err != nil {
			return UpsertResult{}, err
		}
		return c.UpdateDocument(ctx, pageID, doc, true, true)
	}
	pageID, err := c.createChildPageMarkdown(ctx, parentPageID, doc.Title, effectiveMarkdown)
	if err != nil {
		return UpsertResult{}, err
	}
	verified, err := c.getCompletePageMarkdown(ctx, pageID, false, map[string]bool{})
	if err != nil {
		return UpsertResult{}, err
	}
	if err := c.verifyCreatedChildMarkdown(ctx, pageID, doc.Title, effectiveMarkdown, verified.Markdown); err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{
		PageID:     pageID,
		PageURL:    notionPageURL(pageID),
		Created:    true,
		BlockCount: len(doc.Blocks),
	}, nil
}

func (c *Client) createChildPageMarkdown(ctx context.Context, parentPageID string, title string, markdown string) (string, error) {
	payload := map[string]any{
		"parent":   map[string]any{"type": "page_id", "page_id": parentPageID},
		"markdown": NormalizeMarkdown(markdown),
		"properties": map[string]any{
			"title": map[string]any{
				"title": []any{richTextObjectFromInline(Inline{Text: title})},
			},
		},
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.request(ctx, http.MethodPost, "/pages", payload, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.ID) == "" {
		return "", fmt.Errorf("notion create page returned no id")
	}
	return resp.ID, nil
}

func (c *Client) updatePageTitle(ctx context.Context, pageID string, title string) error {
	payload := map[string]any{
		"properties": map[string]any{
			"title": map[string]any{
				"title": []any{richTextObjectFromInline(Inline{Text: title})},
			},
		},
	}
	return c.request(ctx, http.MethodPatch, "/pages/"+pageID, payload, nil)
}

func (c *Client) appendPageContent(ctx context.Context, pageID string, blocks []Block) error {
	return c.appendPageContentAtPosition(ctx, pageID, blocks, appendPosition{})
}

func (c *Client) appendPageContentAtPosition(ctx context.Context, pageID string, blocks []Block, position appendPosition) error {
	payloadBlocks, err := c.convertBlocks(ctx, blocks)
	if err != nil {
		return err
	}
	nextPosition := position
	for start := 0; start < len(payloadBlocks); start += 100 {
		end := start + 100
		if end > len(payloadBlocks) {
			end = len(payloadBlocks)
		}
		payload := map[string]any{"children": payloadBlocks[start:end]}
		if nextPosition.Kind != "" {
			payload["position"] = nextPosition.payload()
		}
		var resp struct {
			Results []notionBlock `json:"results"`
		}
		if err := c.request(ctx, http.MethodPatch, "/blocks/"+pageID+"/children", payload, &resp); err != nil {
			return err
		}
		if end >= len(payloadBlocks) {
			continue
		}
		lastID := lastBlockID(resp.Results)
		if lastID == "" {
			return fmt.Errorf("notion append children returned no block ids for continued insertion")
		}
		nextPosition = appendPosition{Kind: "after_block", AfterBlockID: lastID}
	}
	return nil
}

func (c *Client) verifyCreatedChildMarkdown(ctx context.Context, pageID string, expectedTitle string, expectedMarkdown string, actualMarkdown string) error {
	actualTitle, err := c.pageTitle(ctx, pageID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(actualTitle) != strings.TrimSpace(expectedTitle) {
		return fmt.Errorf("notion child-page title verification failed for %s: expected %q, got %q", pageID, strings.TrimSpace(expectedTitle), strings.TrimSpace(actualTitle))
	}

	expectedBody := comparableMarkdown(markdownWithoutTitle(expectedMarkdown))
	actualBody := comparableMarkdown(markdownWithoutTitle(actualMarkdown))
	if expectedBody != actualBody {
		return fmt.Errorf("notion markdown verification failed for %s: %s", pageID, markdownDiffSummary(expectedBody, actualBody))
	}
	return nil
}

func (c *Client) replacePageContentSafely(ctx context.Context, pageID string, blocks []Block) error {
	existing, err := c.listBlockChildren(ctx, pageID)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return c.appendPageContent(ctx, pageID, blocks)
	}
	for _, block := range existing {
		if block.Type == "child_page" || block.Type == "child_database" {
			return fmt.Errorf("refusing safe block refresh for %s because it contains %s blocks", pageID, block.Type)
		}
	}
	if err := c.appendPageContent(ctx, pageID, blocks); err != nil {
		return err
	}
	return c.archiveBlocks(ctx, existing)
}

func (c *Client) replacePageSectionSafely(ctx context.Context, pageID string, section BlockSection) error {
	existing, err := c.listBlockChildren(ctx, pageID)
	if err != nil {
		return err
	}
	target, err := findBlockSection(existing, section.Heading, section.Level)
	if err != nil {
		return err
	}
	if err := refuseSectionReplacementForProtectedBlocks(pageID, existing[target.Start:target.End]); err != nil {
		return err
	}

	insertPosition := appendPosition{Kind: "start"}
	if target.Start > 0 {
		insertPosition = appendPosition{Kind: "after_block", AfterBlockID: existing[target.Start-1].ID}
	}
	if err := c.appendPageContentAtPosition(ctx, pageID, section.Blocks, insertPosition); err != nil {
		return err
	}
	if err := c.archiveBlocks(ctx, existing[target.Start:target.End]); err != nil {
		return err
	}

	updated, err := c.listBlockChildren(ctx, pageID)
	if err != nil {
		return err
	}
	verified, err := findBlockSection(updated, section.Heading, section.Level)
	if err != nil {
		return err
	}
	if err := verifySectionBlocks(updated[verified.Start:verified.End], section.Blocks); err != nil {
		return err
	}
	return nil
}

func (c *Client) GetPageMarkdown(ctx context.Context, pageID string) (PageMarkdown, error) {
	return c.GetPageMarkdownWithOptions(ctx, pageID, false)
}

func (c *Client) GetPageMarkdownWithOptions(ctx context.Context, pageID string, includeTranscript bool) (PageMarkdown, error) {
	var resp PageMarkdown
	path := "/pages/" + pageID + "/markdown"
	if includeTranscript {
		path += "?include_transcript=true"
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return PageMarkdown{}, err
	}
	resp.Markdown = NormalizeMarkdown(resp.Markdown)
	return resp, nil
}

func (c *Client) getCompletePageMarkdown(ctx context.Context, pageID string, includeTranscript bool, seen map[string]bool) (PageMarkdown, error) {
	pageMarkdown, err := c.GetPageMarkdownWithOptions(ctx, pageID, includeTranscript)
	if err != nil {
		return PageMarkdown{}, err
	}
	if seen[pageID] {
		return pageMarkdown, nil
	}
	seen[pageID] = true
	return c.resolveUnknownMarkdown(ctx, pageMarkdown, includeTranscript, seen)
}

func (c *Client) resolveUnknownMarkdown(ctx context.Context, pageMarkdown PageMarkdown, includeTranscript bool, seen map[string]bool) (PageMarkdown, error) {
	if len(pageMarkdown.UnknownBlockIDs) == 0 {
		return pageMarkdown, nil
	}

	replacements := make([]string, 0, len(pageMarkdown.UnknownBlockIDs))
	unresolved := make([]string, 0)
	truncated := false

	for _, blockID := range pageMarkdown.UnknownBlockIDs {
		if seen[blockID] {
			replacement, err := c.renderUnsupportedBlockMarkdown(ctx, blockID)
			if err != nil {
				unresolved = append(unresolved, blockID)
				truncated = true
			} else {
				replacements = append(replacements, replacement)
			}
			continue
		}
		resolved, err := c.getCompletePageMarkdown(ctx, blockID, includeTranscript, seen)
		if err != nil {
			if isRecoverableUnknownBlockError(err) {
				replacement, replacementErr := c.renderUnsupportedBlockMarkdown(ctx, blockID)
				if replacementErr != nil {
					replacements = append(replacements, fmt.Sprintf(`<unknown-block id="%s"/>`, strings.TrimSpace(blockID)))
				} else {
					replacements = append(replacements, replacement)
				}
				continue
			}
			return PageMarkdown{}, err
		}
		replacements = append(replacements, strings.TrimSpace(resolved.Markdown))
		if resolved.Truncated || len(resolved.UnknownBlockIDs) > 0 {
			unresolved = append(unresolved, resolved.UnknownBlockIDs...)
			unresolved = append(unresolved, blockID)
			truncated = true
		}
	}

	markdown := pageMarkdown.Markdown
	used := 0
	markdown = unknownMarkdownTagRE.ReplaceAllStringFunc(markdown, func(_ string) string {
		if used >= len(replacements) {
			truncated = true
			return ""
		}
		replacement := replacements[used]
		used++
		if replacement == "" {
			truncated = true
		}
		return replacement
	})
	if used < len(replacements) {
		truncated = true
	}

	resolvedMarkdown := PageMarkdown{
		ID:              pageMarkdown.ID,
		Markdown:        NormalizeMarkdown(markdown),
		Truncated:       truncated,
		UnknownBlockIDs: dedupeStrings(unresolved),
	}
	return resolvedMarkdown, nil
}

func (c *Client) renderUnsupportedBlockMarkdown(ctx context.Context, blockID string) (string, error) {
	block, err := c.getBlock(ctx, blockID)
	if err != nil {
		return "", err
	}
	if block.HasChildren {
		children, err := c.listBlockChildren(ctx, blockID)
		if err != nil {
			return "", err
		}
		block.Children = children
	}
	return strings.TrimSpace(strings.Join(renderBlocksMarkdown([]notionBlock{block}, 0, ""), "\n")), nil
}

func isRecoverableUnknownBlockError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "object_not_found") || strings.Contains(message, "restricted_resource")
}

func (c *Client) replacePageMarkdown(ctx context.Context, pageID string, markdown string) (PageMarkdown, error) {
	var resp PageMarkdown
	payload := map[string]any{
		"type": "replace_content",
		"replace_content": map[string]any{
			"new_str": NormalizeMarkdown(markdown),
		},
	}
	if err := c.request(ctx, http.MethodPatch, "/pages/"+pageID+"/markdown", payload, &resp); err != nil {
		return PageMarkdown{}, err
	}
	resp.Markdown = NormalizeMarkdown(resp.Markdown)
	return resp, nil
}

func (c *Client) updatePageMarkdownSearchReplace(ctx context.Context, pageID string, updates []ContentUpdate) (PageMarkdown, error) {
	contentUpdates := make([]map[string]any, 0, len(updates))
	for _, update := range updates {
		item := map[string]any{
			"old_str": update.OldStr,
			"new_str": update.NewStr,
		}
		if update.ReplaceAllMatches {
			item["replace_all_matches"] = true
		}
		contentUpdates = append(contentUpdates, item)
	}

	var resp PageMarkdown
	payload := map[string]any{
		"type": "update_content",
		"update_content": map[string]any{
			"content_updates": contentUpdates,
		},
	}
	if err := c.request(ctx, http.MethodPatch, "/pages/"+pageID+"/markdown", payload, &resp); err != nil {
		return PageMarkdown{}, err
	}
	resp.Markdown = NormalizeMarkdown(resp.Markdown)
	return resp, nil
}

func (c *Client) convertBlocks(ctx context.Context, blocks []Block) ([]map[string]any, error) {
	payload := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		converted, err := c.convertBlock(ctx, block)
		if err != nil {
			return nil, err
		}
		if converted != nil {
			payload = append(payload, converted)
		}
	}
	return payload, nil
}

func (c *Client) convertBlock(ctx context.Context, block Block) (map[string]any, error) {
	if isAssetBlockKind(block.Kind) {
		return c.convertMediaBlock(ctx, block)
	}

	switch block.Kind {
	case "divider":
		return map[string]any{
			"object":  "block",
			"type":    "divider",
			"divider": map[string]any{},
		}, nil
	case "equation":
		return map[string]any{
			"object": "block",
			"type":   "equation",
			"equation": map[string]any{
				"expression": block.Text,
			},
		}, nil
	case "to_do":
		body := map[string]any{
			"rich_text": richText(block),
			"checked":   block.Checked,
		}
		if color := normalizeNotionColor(block.Color); color != "" {
			body["color"] = color
		}
		children, err := c.convertBlocks(ctx, block.Children)
		if err != nil {
			return nil, err
		}
		if len(children) > 0 {
			body["children"] = children
		}
		return map[string]any{
			"object": "block",
			"type":   "to_do",
			"to_do":  body,
		}, nil
	case "callout":
		callout := map[string]any{
			"rich_text": richText(block),
		}
		if strings.TrimSpace(block.Icon) != "" {
			callout["icon"] = map[string]any{
				"type":  "emoji",
				"emoji": block.Icon,
			}
		}
		if color := normalizeNotionColor(block.Color); color != "" {
			callout["color"] = color
		}
		children, err := c.convertBlocks(ctx, block.Children)
		if err != nil {
			return nil, err
		}
		if len(children) > 0 {
			callout["children"] = children
		}
		return map[string]any{
			"object":  "block",
			"type":    "callout",
			"callout": callout,
		}, nil
	case "toggle":
		toggle := map[string]any{
			"rich_text": richText(block),
		}
		if color := normalizeNotionColor(block.Color); color != "" {
			toggle["color"] = color
		}
		children, err := c.convertBlocks(ctx, block.Children)
		if err != nil {
			return nil, err
		}
		if len(children) > 0 {
			toggle["children"] = children
		}
		return map[string]any{
			"object": "block",
			"type":   "toggle",
			"toggle": toggle,
		}, nil
	case "table":
		tableRows, err := c.convertTableRows(block)
		if err != nil {
			return nil, err
		}
		width := 0
		if len(block.TableRows) > 0 {
			width = len(block.TableRows[0])
		}
		return map[string]any{
			"object": "block",
			"type":   "table",
			"table": map[string]any{
				"table_width":       width,
				"has_column_header": block.TableHeader,
				"has_row_header":    block.TableRowHead,
				"children":          tableRows,
			},
		}, nil
	case "page_ref":
		return map[string]any{
			"object": "block",
			"type":   "link_to_page",
			"link_to_page": map[string]any{
				"type":    "page_id",
				"page_id": strings.TrimSpace(block.RefID),
			},
		}, nil
	case "database_ref":
		return map[string]any{
			"object": "block",
			"type":   "link_to_page",
			"link_to_page": map[string]any{
				"type":        "database_id",
				"database_id": strings.TrimSpace(block.RefID),
			},
		}, nil
	default:
		body := map[string]any{"rich_text": richText(block)}
		if block.Kind == "code" {
			body["language"] = firstNonEmpty(block.CodeLanguage, "plain text")
		}
		if color := normalizeNotionColor(block.Color); color != "" {
			body["color"] = color
		}
		children, err := c.convertBlocks(ctx, block.Children)
		if err != nil {
			return nil, err
		}
		if len(children) > 0 {
			body["children"] = children
		}
		return map[string]any{
			"object":   "block",
			"type":     block.Kind,
			block.Kind: body,
		}, nil
	}
}

func (c *Client) convertMediaBlock(ctx context.Context, block Block) (map[string]any, error) {
	path := strings.TrimSpace(block.AssetPath)
	if path == "" {
		return nil, nil
	}
	body := map[string]any{
		"caption": []any{richTextObjectFromInline(Inline{Text: block.Caption})},
	}
	if parsed, err := url.Parse(path); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		body["type"] = "external"
		body["external"] = map[string]any{"url": path}
	} else {
		fileUploadID, err := c.uploadFile(ctx, path)
		if err != nil {
			return nil, err
		}
		body["type"] = "file_upload"
		body["file_upload"] = map[string]any{"id": fileUploadID}
	}
	return map[string]any{"object": "block", "type": block.Kind, block.Kind: body}, nil
}

func (c *Client) convertTableRows(block Block) ([]map[string]any, error) {
	rows := make([]map[string]any, 0, len(block.TableRows))
	for _, row := range block.TableRows {
		cells := make([][]any, 0, len(row))
		for _, cell := range row {
			cells = append(cells, richTextSegments(cell))
		}
		rows = append(rows, map[string]any{
			"object": "block",
			"type":   "table_row",
			"table_row": map[string]any{
				"cells": cells,
			},
		})
	}
	return rows, nil
}

func normalizeNotionColor(color string) string {
	switch strings.TrimSpace(strings.ToLower(color)) {
	case "", "default":
		return ""
	case "gray", "brown", "orange", "yellow", "green", "blue", "purple", "pink", "red":
		return strings.TrimSpace(strings.ToLower(color))
	case "gray_bg":
		return "gray_background"
	case "brown_bg":
		return "brown_background"
	case "orange_bg":
		return "orange_background"
	case "yellow_bg":
		return "yellow_background"
	case "green_bg":
		return "green_background"
	case "blue_bg":
		return "blue_background"
	case "purple_bg":
		return "purple_background"
	case "pink_bg":
		return "pink_background"
	case "red_bg":
		return "red_background"
	case "gray_background", "brown_background", "orange_background", "yellow_background", "green_background", "blue_background", "purple_background", "pink_background", "red_background":
		return strings.TrimSpace(strings.ToLower(color))
	default:
		return ""
	}
}

func (c *Client) uploadFile(ctx context.Context, path string) (string, error) {
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	filename := filepath.Base(path)
	contentType := detectContentType(path)

	mode := "single_part"
	partCount := 1
	if info.Size() > maxSinglePartUploadSize {
		mode = "multi_part"
		partCount = int((info.Size() + multipartUploadPartSize - 1) / multipartUploadPartSize)
	}

	createdID, err := c.createFileUpload(ctx, filename, contentType, mode, partCount)
	if err != nil {
		return "", err
	}

	if mode == "single_part" {
		if err := c.sendFileUpload(ctx, createdID, path, filename, contentType); err != nil {
			return "", err
		}
		return createdID, nil
	}

	for partNumber := 1; partNumber <= partCount; partNumber++ {
		offset := int64(partNumber-1) * multipartUploadPartSize
		length := multipartUploadPartSize
		if remaining := info.Size() - offset; remaining < length {
			length = remaining
		}
		if err := c.sendFileUploadPart(ctx, createdID, path, filename, contentType, partNumber, offset, length); err != nil {
			return "", err
		}
	}
	if err := c.completeFileUpload(ctx, createdID); err != nil {
		return "", err
	}
	return createdID, nil
}

func (c *Client) createFileUpload(ctx context.Context, filename string, contentType string, mode string, partCount int) (string, error) {
	var created struct {
		ID string `json:"id"`
	}
	payload := map[string]any{
		"mode":         mode,
		"filename":     filename,
		"content_type": contentType,
	}
	if mode == "multi_part" {
		payload["number_of_parts"] = partCount
	}
	if err := c.request(ctx, http.MethodPost, "/file_uploads", payload, &created); err != nil {
		return "", err
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", fmt.Errorf("notion create file upload returned no id")
	}
	return created.ID, nil
}

func (c *Client) sendFileUpload(ctx context.Context, fileUploadID string, filePath string, uploadName string, contentType string) error {
	return c.sendFileUploadPart(ctx, fileUploadID, filePath, uploadName, contentType, 0, 0, -1)
}

func (c *Client) sendFileUploadPart(ctx context.Context, fileUploadID string, filePath string, uploadName string, contentType string, partNumber int, offset int64, length int64) error {
	file, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return err
	}
	defer file.Close()
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return err
		}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if partNumber > 0 {
		if err := writer.WriteField("part_number", strconv.Itoa(partNumber)); err != nil {
			return err
		}
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filepath.Base(uploadName)))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	if length >= 0 {
		if _, err := io.CopyN(part, file, length); err != nil {
			return err
		}
	} else {
		if _, err := io.Copy(part, file); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/file_uploads/"+fileUploadID+"/send", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", notionVersion)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.doRequestWithRetry(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notion API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var uploaded struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&uploaded); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if !isAcceptedFileUploadStatus(uploaded.Status, partNumber > 0) {
		return fmt.Errorf("notion file upload status = %q", uploaded.Status)
	}
	return nil
}

func (c *Client) completeFileUpload(ctx context.Context, fileUploadID string) error {
	var uploaded struct {
		Status string `json:"status"`
	}
	if err := c.request(ctx, http.MethodPost, "/file_uploads/"+fileUploadID+"/complete", nil, &uploaded); err != nil {
		return err
	}
	if !isAcceptedCompletedUploadStatus(uploaded.Status) {
		return fmt.Errorf("notion file upload status = %q", uploaded.Status)
	}
	return nil
}

func isAcceptedFileUploadStatus(status string, allowPending bool) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", "uploaded":
		return true
	case "pending", "processing", "complete", "completed":
		return allowPending
	default:
		return false
	}
}

func isAcceptedCompletedUploadStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", "uploaded", "complete", "completed":
		return true
	default:
		return false
	}
}

func (c *Client) pageTitle(ctx context.Context, pageID string) (string, error) {
	var resp struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Title []struct {
				PlainText string `json:"plain_text"`
			} `json:"title,omitempty"`
		} `json:"properties"`
	}
	if err := c.request(ctx, http.MethodGet, "/pages/"+pageID, nil, &resp); err != nil {
		return "", err
	}
	for _, property := range resp.Properties {
		if property.Type != "title" {
			continue
		}
		var parts []string
		for _, item := range property.Title {
			parts = append(parts, item.PlainText)
		}
		title := strings.TrimSpace(strings.Join(parts, ""))
		if title != "" {
			return title, nil
		}
	}
	return "Untitled", nil
}

func (c *Client) getBlock(ctx context.Context, blockID string) (notionBlock, error) {
	var block notionBlock
	if err := c.request(ctx, http.MethodGet, "/blocks/"+blockID, nil, &block); err != nil {
		return notionBlock{}, err
	}
	return block, nil
}

func (c *Client) listBlockChildren(ctx context.Context, blockID string) ([]notionBlock, error) {
	var all []notionBlock
	cursor := ""
	for {
		path := fmt.Sprintf("/blocks/%s/children?page_size=100", blockID)
		if cursor != "" {
			path += "&start_cursor=" + url.QueryEscape(cursor)
		}
		var resp struct {
			Results    []notionBlock `json:"results"`
			HasMore    bool          `json:"has_more"`
			NextCursor string        `json:"next_cursor"`
		}
		if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Results...)
		if !resp.HasMore || resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	for i := range all {
		if !all[i].HasChildren {
			continue
		}
		children, err := c.listBlockChildren(ctx, all[i].ID)
		if err != nil {
			return nil, err
		}
		all[i].Children = children
	}
	return all, nil
}

func (c *Client) archiveBlocks(ctx context.Context, blocks []notionBlock) error {
	if len(blocks) == 0 {
		return nil
	}

	workerCount := blockArchiveConcurrency
	if workerCount > len(blocks) {
		workerCount = len(blocks)
	}

	jobs := make(chan notionBlock)
	errs := make(chan error, len(blocks))

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for block := range jobs {
				if err := c.archiveBlock(ctx, block); err != nil {
					errs <- err
				}
			}
		}()
	}

	for _, block := range blocks {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- block:
		}
	}
	close(jobs)
	wg.Wait()
	close(errs)

	collected := make([]error, 0, len(blocks))
	for err := range errs {
		collected = append(collected, err)
	}
	return errorsJoin(collected)
}

func (c *Client) archiveBlock(ctx context.Context, block notionBlock) error {
	deleteCtx, cancel := context.WithTimeout(ctx, blockArchiveTimeout)
	defer cancel()

	if err := c.request(deleteCtx, http.MethodDelete, "/blocks/"+block.ID, nil, nil); err != nil {
		if strings.Contains(err.Error(), "Can't edit block that is archived") {
			return nil
		}
		return fmt.Errorf("archive block %s: %w", block.ID, err)
	}
	return nil
}

func (p appendPosition) payload() map[string]any {
	switch p.Kind {
	case "start":
		return map[string]any{"type": "start"}
	case "after_block":
		return map[string]any{
			"type":        "after_block",
			"after_block": map[string]any{"id": strings.TrimSpace(p.AfterBlockID)},
		}
	default:
		return nil
	}
}

func lastBlockID(blocks []notionBlock) string {
	for i := len(blocks) - 1; i >= 0; i-- {
		if strings.TrimSpace(blocks[i].ID) != "" {
			return strings.TrimSpace(blocks[i].ID)
		}
	}
	return ""
}

func (c *Client) request(ctx context.Context, method string, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", notionVersion)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.doRequestWithRetry(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notion API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) doRequestWithRetry(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= maxRequestTries; attempt++ {
		cloned := req.Clone(req.Context())
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			cloned.Body = body
		}

		resp, err := c.httpClient.Do(cloned)
		if err == nil && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return resp, nil
		}
		if err != nil {
			lastErr = err
		} else {
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("notion API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		if attempt == maxRequestTries {
			break
		}
		if err := sleepWithContext(req.Context(), time.Duration(attempt)*time.Second); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func richText(block Block) []any {
	if block.Kind == "code" || len(block.Inlines) == 0 {
		return richTextSegments([]Inline{{Text: block.Text}})
	}
	return richTextSegments(block.Inlines)
}

func richTextSegments(inlines []Inline) []any {
	if len(inlines) == 0 {
		return []any{richTextObjectFromInline(Inline{Text: ""})}
	}
	parts := make([]any, 0, len(inlines))
	for _, inline := range inlines {
		text := inline.Text
		if text == "" {
			continue
		}
		for len(text) > 0 {
			chunk := text
			if len(chunk) > 1800 {
				chunk = chunk[:1800]
				if lastSpace := strings.LastIndex(chunk, " "); lastSpace > 0 && !inline.Code {
					chunk = chunk[:lastSpace]
				}
			}
			inlineChunk := inline
			inlineChunk.Text = chunk
			parts = append(parts, richTextObjectFromInline(inlineChunk))
			text = strings.TrimPrefix(text, chunk)
		}
	}
	if len(parts) == 0 {
		return []any{richTextObjectFromInline(Inline{Text: ""})}
	}
	return parts
}

func richTextObjectFromInline(inline Inline) map[string]any {
	obj := map[string]any{}
	if strings.TrimSpace(inline.MentionType) != "" && strings.TrimSpace(inline.MentionID) != "" {
		obj["type"] = "mention"
		mention := map[string]any{"type": inline.MentionType}
		switch inline.MentionType {
		case "page":
			mention["page"] = map[string]any{"id": strings.TrimSpace(inline.MentionID)}
		case "database":
			mention["database"] = map[string]any{"id": strings.TrimSpace(inline.MentionID)}
		case "user":
			mention["user"] = map[string]any{"id": strings.TrimSpace(inline.MentionID)}
		case "date":
			mention["date"] = map[string]any{
				"start": strings.TrimSpace(inline.MentionStart),
			}
			if date, ok := mention["date"].(map[string]any); ok {
				if strings.TrimSpace(inline.MentionEnd) != "" {
					date["end"] = strings.TrimSpace(inline.MentionEnd)
				}
				if strings.TrimSpace(inline.MentionTimeZone) != "" {
					date["time_zone"] = strings.TrimSpace(inline.MentionTimeZone)
				}
			}
		case "link_preview":
			mention["link_preview"] = map[string]any{"url": strings.TrimSpace(inline.MentionURL)}
		case "template_mention":
			template := map[string]any{
				"type": strings.TrimSpace(inline.MentionTemplateType),
			}
			switch strings.TrimSpace(inline.MentionTemplateType) {
			case "template_mention_date":
				template["template_mention_date"] = strings.TrimSpace(inline.MentionTemplateValue)
			case "template_mention_user":
				template["template_mention_user"] = strings.TrimSpace(inline.MentionTemplateValue)
			}
			mention["template_mention"] = template
		}
		obj["mention"] = mention
	} else {
		obj["type"] = "text"
		obj["text"] = map[string]any{
			"content": inline.Text,
		}
		if strings.TrimSpace(inline.URL) != "" {
			textObj, _ := obj["text"].(map[string]any)
			textObj["link"] = map[string]any{"url": strings.TrimSpace(inline.URL)}
		}
	}
	obj["annotations"] = map[string]any{
		"bold":          inline.Bold,
		"italic":        inline.Italic,
		"strikethrough": inline.Strikethrough,
		"underline":     inline.Underline,
		"code":          inline.Code,
		"color":         firstNonEmpty(normalizeNotionColor(inline.Color), "default"),
	}
	return obj
}

func joinRichText(items []richTextItem) string {
	var parts []string
	for _, item := range items {
		text := item.PlainText
		text = renderRichTextItem(text, item)
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func renderRichTextItem(text string, item richTextItem) string {
	if item.Mention != nil {
		switch item.Mention.Type {
		case "page":
			if item.Mention.Page != nil {
				label := strings.TrimSpace(text)
				if label == "" {
					label = strings.TrimSpace(item.Mention.Page.ID)
				}
				return fmt.Sprintf(`<mention-page url="%s">%s</mention-page>`, notionPageURL(item.Mention.Page.ID), label)
			}
		case "database":
			if item.Mention.Database != nil {
				label := strings.TrimSpace(text)
				if label == "" {
					label = strings.TrimSpace(item.Mention.Database.ID)
				}
				return fmt.Sprintf(`<mention-database url="%s">%s</mention-database>`, notionPageURL(item.Mention.Database.ID), label)
			}
		case "user":
			if item.Mention.User != nil {
				label := strings.TrimSpace(text)
				if label == "" {
					label = strings.TrimSpace(item.Mention.User.ID)
				}
				return fmt.Sprintf(`<mention-user id="%s">%s</mention-user>`, strings.TrimSpace(item.Mention.User.ID), label)
			}
		case "date":
			if item.Mention.Date != nil {
				label := strings.TrimSpace(text)
				if label == "" {
					label = strings.TrimSpace(item.Mention.Date.Start)
				}
				attrs := []string{fmt.Sprintf(`start="%s"`, strings.TrimSpace(item.Mention.Date.Start))}
				if strings.TrimSpace(item.Mention.Date.End) != "" {
					attrs = append(attrs, fmt.Sprintf(`end="%s"`, strings.TrimSpace(item.Mention.Date.End)))
				}
				if strings.TrimSpace(item.Mention.Date.TimeZone) != "" {
					attrs = append(attrs, fmt.Sprintf(`timezone="%s"`, strings.TrimSpace(item.Mention.Date.TimeZone)))
				}
				return fmt.Sprintf(`<mention-date %s>%s</mention-date>`, strings.Join(attrs, " "), label)
			}
		case "link_preview":
			if item.Mention.LinkPreview != nil {
				label := strings.TrimSpace(text)
				if label == "" {
					label = strings.TrimSpace(item.Mention.LinkPreview.URL)
				}
				return fmt.Sprintf(`<mention-link-preview url="%s">%s</mention-link-preview>`, strings.TrimSpace(item.Mention.LinkPreview.URL), label)
			}
		case "template_mention":
			if item.Mention.TemplateMention != nil {
				label := strings.TrimSpace(text)
				templateType := strings.TrimSpace(item.Mention.TemplateMention.Type)
				templateValue := ""
				switch templateType {
				case "template_mention_date":
					templateValue = strings.TrimSpace(item.Mention.TemplateMention.TemplateMentionDate)
				case "template_mention_user":
					templateValue = strings.TrimSpace(item.Mention.TemplateMention.TemplateMentionUser)
				}
				if label == "" {
					label = firstNonEmpty(templateValue, templateType)
				}
				return fmt.Sprintf(`<mention-template type="%s" value="%s">%s</mention-template>`, templateType, templateValue, label)
			}
		}
	}
	if text == "" {
		return ""
	}
	if item.Annotations != nil && item.Annotations.Code {
		if item.Href != "" {
			return fmt.Sprintf("[%s](%s)", "`"+text+"`", item.Href)
		}
		return "`" + text + "`"
	}
	if item.Annotations != nil {
		if item.Annotations.Bold {
			text = "**" + text + "**"
		}
		if item.Annotations.Italic {
			text = "*" + text + "*"
		}
		if item.Annotations.Strikethrough {
			text = "~~" + text + "~~"
		}
		if item.Annotations.Underline {
			text = `<span underline="true">` + text + `</span>`
		}
		if color := strings.TrimSpace(item.Annotations.Color); color != "" && color != "default" {
			text = `<span color="` + color + `">` + text + `</span>`
		}
	}
	if item.Href != "" {
		text = fmt.Sprintf("[%s](%s)", text, item.Href)
	}
	return text
}

func renderBlocksAsMarkdown(title string, blocks []notionBlock) string {
	body := []string{"# " + strings.TrimSpace(title), ""}
	body = append(body, renderBlocksMarkdown(blocks, 0, strings.TrimSpace(title))...)
	return strings.TrimSpace(strings.Join(compactMarkdown(body), "\n"))
}

func renderBlocksMarkdown(blocks []notionBlock, depth int, pageTitle string) []string {
	body := make([]string, 0)
	for _, block := range blocks {
		switch block.Type {
		case "paragraph":
			if block.Paragraph != nil {
				body = append(body, joinRichText(block.Paragraph.RichText), "")
			}
		case "heading_1":
			if block.Heading1 != nil {
				heading := joinRichText(block.Heading1.RichText)
				if strings.TrimSpace(heading) == pageTitle {
					continue
				}
				body = append(body, "# "+heading, "")
			}
		case "heading_2":
			if block.Heading2 != nil {
				body = append(body, "## "+joinRichText(block.Heading2.RichText), "")
			}
		case "heading_3":
			if block.Heading3 != nil {
				body = append(body, "### "+joinRichText(block.Heading3.RichText), "")
			}
		case "bulleted_list_item":
			if block.BulletedListItem != nil {
				body = append(body, "- "+joinRichText(block.BulletedListItem.RichText))
			}
		case "numbered_list_item":
			if block.NumberedListItem != nil {
				body = append(body, "1. "+joinRichText(block.NumberedListItem.RichText))
			}
		case "to_do":
			if block.ToDo != nil {
				prefix := "- [ ] "
				if block.ToDo.Checked {
					prefix = "- [x] "
				}
				body = append(body, prefix+joinRichText(block.ToDo.RichText))
			}
		case "quote":
			if block.Quote != nil {
				body = append(body, "> "+joinRichText(block.Quote.RichText), "")
			}
		case "divider":
			body = append(body, "---", "")
		case "callout":
			if block.Callout != nil {
				body = append(body, renderCalloutBlockMarkdown(block)...)
			}
		case "toggle":
			if block.Toggle != nil {
				body = append(body, "<details>", "<summary>"+joinRichText(block.Toggle.RichText)+"</summary>", "")
				body = append(body, renderBlocksMarkdown(block.Children, depth+1, pageTitle)...)
				body = append(body, "</details>", "")
			}
		case "table":
			body = append(body, renderTableBlockMarkdown(block)...)
		case "table_row":
			continue
		case "template":
			body = append(body, renderTemplateBlockMarkdown(block, pageTitle)...)
		case "synced_block":
			body = append(body, renderSyncedBlockMarkdown(block, pageTitle)...)
		case "breadcrumb":
			body = append(body, "<breadcrumb/>", "")
		case "table_of_contents":
			body = append(body, renderTableOfContentsBlockMarkdown(block), "")
		case "column_list":
			body = append(body, renderColumnListBlockMarkdown(block, pageTitle)...)
		case "column":
			body = append(body, renderColumnBlockMarkdown(block, pageTitle)...)
		case "embed":
			body = append(body, renderURLBlockMarkdown("embed", firstNonEmpty(blockURL(block), ""), ""), "")
		case "bookmark":
			body = append(body, renderURLBlockMarkdown("bookmark", firstNonEmpty(blockURL(block), ""), ""), "")
		case "link_preview":
			body = append(body, renderURLBlockMarkdown("link-preview", firstNonEmpty(blockURL(block), ""), ""), "")
		case "code":
			if block.Code != nil {
				body = append(body, "```"+strings.TrimSpace(block.Code.Language), joinRichText(block.Code.RichText), "```", "")
			}
		case "equation":
			if block.Equation != nil {
				body = append(body, "$$", strings.TrimSpace(block.Equation.Expression), "$$", "")
			}
		case "image", "file", "pdf", "audio", "video":
			body = append(body, renderMediaBlockMarkdown(block), "")
		case "child_page":
			if block.ChildPage != nil {
				body = append(body, fmt.Sprintf(`<page url="%s">%s</page>`, notionPageURL(block.ID), strings.TrimSpace(block.ChildPage.Title)), "")
			}
		case "child_database":
			if block.ChildDatabase != nil {
				body = append(body, fmt.Sprintf(`<database url="%s">%s</database>`, notionPageURL(block.ID), strings.TrimSpace(block.ChildDatabase.Title)), "")
			}
		case "link_to_page":
			if block.LinkToPage != nil {
				if block.LinkToPage.Type == "page_id" {
					body = append(body, fmt.Sprintf(`<page url="%s">%s</page>`, notionPageURL(block.LinkToPage.PageID), strings.TrimSpace(block.TextFromLinkRef())), "")
				} else if block.LinkToPage.Type == "database_id" {
					body = append(body, fmt.Sprintf(`<database url="%s">%s</database>`, notionPageURL(block.LinkToPage.DatabaseID), strings.TrimSpace(block.TextFromLinkRef())), "")
				}
			}
		}
	}
	return body
}

func renderMediaBlockMarkdown(block notionBlock) string {
	data := mediaBlockData(block)
	if data == nil {
		return ""
	}
	caption := joinRichText(data.Caption)
	switch block.Type {
	case "image":
		switch data.Type {
		case "external":
			if data.External != nil {
				return fmt.Sprintf("![%s](%s)", caption, strings.TrimSpace(data.External.URL))
			}
		case "file":
			if data.File != nil {
				return fmt.Sprintf("![%s](%s)", caption, strings.TrimSpace(data.File.URL))
			}
		}
		return fmt.Sprintf(`<unknown url="%s" alt="image"/>`, notionPageURL(block.ID))
	}

	switch data.Type {
	case "external":
		if data.External != nil {
			return fmt.Sprintf(`<%s src="%s">%s</%s>`, block.Type, strings.TrimSpace(data.External.URL), caption, block.Type)
		}
	case "file":
		if data.File != nil {
			return fmt.Sprintf(`<%s src="%s">%s</%s>`, block.Type, strings.TrimSpace(data.File.URL), caption, block.Type)
		}
	}
	return fmt.Sprintf(`<unknown url="%s" alt="%s"/>`, notionPageURL(block.ID), block.Type)
}

func blockURL(block notionBlock) string {
	switch block.Type {
	case "embed":
		if block.Embed != nil {
			return strings.TrimSpace(block.Embed.URL)
		}
	case "bookmark":
		if block.Bookmark != nil {
			return strings.TrimSpace(block.Bookmark.URL)
		}
	case "link_preview":
		if block.LinkPreview != nil {
			return strings.TrimSpace(block.LinkPreview.URL)
		}
	}
	return ""
}

func renderURLBlockMarkdown(tag string, value string, label string) string {
	value = strings.TrimSpace(value)
	label = strings.TrimSpace(label)
	if value == "" {
		if label == "" {
			return ""
		}
		return fmt.Sprintf(`<%s>%s</%s>`, tag, label, tag)
	}
	if label == "" {
		return fmt.Sprintf(`<%s url="%s"/>`, tag, value)
	}
	return fmt.Sprintf(`<%s url="%s">%s</%s>`, tag, value, label, tag)
}

func renderTableOfContentsBlockMarkdown(block notionBlock) string {
	if block.TableOfContents == nil {
		return "<table-of-contents/>"
	}
	if color := strings.TrimSpace(block.TableOfContents.Color); color != "" && color != "default" {
		return fmt.Sprintf(`<table-of-contents color="%s"/>`, color)
	}
	return "<table-of-contents/>"
}

func renderTemplateBlockMarkdown(block notionBlock, pageTitle string) []string {
	label := ""
	if block.Template != nil {
		label = joinRichText(block.Template.RichText)
	}
	if len(block.Children) == 0 {
		return []string{renderURLBlockMarkdown("template", "", label), ""}
	}
	open := "<template>"
	if strings.TrimSpace(label) != "" {
		open = fmt.Sprintf(`<template label="%s">`, label)
	}
	body := []string{open, ""}
	body = append(body, renderBlocksMarkdown(block.Children, 0, pageTitle)...)
	body = append(body, "</template>", "")
	return body
}

func renderSyncedBlockMarkdown(block notionBlock, pageTitle string) []string {
	if block.SyncedBlock != nil && block.SyncedBlock.SyncedFrom != nil && strings.TrimSpace(block.SyncedBlock.SyncedFrom.BlockID) != "" {
		return []string{fmt.Sprintf(`<synced-block block-id="%s"/>`, strings.TrimSpace(block.SyncedBlock.SyncedFrom.BlockID)), ""}
	}
	body := []string{"<synced-block>", ""}
	body = append(body, renderBlocksMarkdown(block.Children, 0, pageTitle)...)
	body = append(body, "</synced-block>", "")
	return body
}

func renderColumnListBlockMarkdown(block notionBlock, pageTitle string) []string {
	body := []string{"<columns>", ""}
	for _, child := range block.Children {
		body = append(body, renderColumnBlockMarkdown(child, pageTitle)...)
	}
	body = append(body, "</columns>", "")
	return body
}

func renderColumnBlockMarkdown(block notionBlock, pageTitle string) []string {
	body := []string{"<column>", ""}
	body = append(body, renderBlocksMarkdown(block.Children, 0, pageTitle)...)
	body = append(body, "</column>", "")
	return body
}

func renderTableBlockMarkdown(block notionBlock) []string {
	if len(block.Children) == 0 {
		return nil
	}
	hasColumnHeader := true
	hasRowHeader := false
	if block.Table != nil {
		hasColumnHeader = block.Table.HasColumnHeader
		hasRowHeader = block.Table.HasRowHeader
	}
	lines := []string{fmt.Sprintf(`<table header-row="%t" header-column="%t">`, hasColumnHeader, hasRowHeader)}
	for _, row := range block.Children {
		if row.TableRow == nil {
			continue
		}
		lines = append(lines, "<tr>")
		for _, cell := range row.TableRow.Cells {
			lines = append(lines, "<td>"+joinRichText(cell)+"</td>")
		}
		lines = append(lines, "</tr>")
	}
	lines = append(lines, "</table>", "")
	return lines
}

func renderCalloutBlockMarkdown(block notionBlock) []string {
	if block.Callout == nil {
		return nil
	}
	icon := ""
	color := ""
	if block.Callout.Icon != nil {
		icon = strings.TrimSpace(block.Callout.Icon.Emoji)
	}
	color = strings.TrimSpace(block.Callout.Color)
	open := `<callout`
	if icon != "" {
		open += fmt.Sprintf(` icon="%s"`, icon)
	}
	if color != "" && color != "default" {
		open += fmt.Sprintf(` color="%s"`, color)
	}
	open += ">"

	lines := []string{open}
	body := joinRichText(block.Callout.RichText)
	if body != "" {
		lines = append(lines, "\t"+body)
	}
	for _, line := range renderBlocksMarkdown(block.Children, 0, "") {
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, "\t"+line)
	}
	lines = append(lines, "</callout>", "")
	return lines
}

func (b notionBlock) TextFromLinkRef() string {
	switch b.LinkToPage.Type {
	case "page_id":
		return strings.TrimSpace(b.LinkToPage.PageID)
	case "database_id":
		return strings.TrimSpace(b.LinkToPage.DatabaseID)
	default:
		return ""
	}
}

func calloutLabelForEmoji(emoji string) string {
	switch strings.TrimSpace(emoji) {
	case "📝":
		return "NOTE"
	case "💡":
		return "TIP"
	case "❗":
		return "IMPORTANT"
	case "⚠️":
		return "WARNING"
	case "🚨":
		return "CAUTION"
	default:
		return "NOTE"
	}
}

func mediaBlockData(block notionBlock) *notionMediaBlockData {
	switch block.Type {
	case "image":
		return block.Image
	case "file":
		return block.File
	case "pdf":
		return block.PDF
	case "audio":
		return block.Audio
	case "video":
		return block.Video
	default:
		return nil
	}
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func compactMarkdown(lines []string) []string {
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " ")
		if line == "" {
			if blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	return out
}

func notionPageURL(pageID string) string {
	return "https://www.notion.so/" + strings.ReplaceAll(strings.TrimSpace(pageID), "-", "")
}

func detectContentType(path string) string {
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func errorsJoin(errs []error) error {
	filtered := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err.Error())
		}
	}
	sort.Strings(filtered)
	if len(filtered) == 0 {
		return nil
	}
	return errors.New(strings.Join(filtered, "; "))
}

func findBlockSection(blocks []notionBlock, heading string, level int) (BlockSection, error) {
	type headingEntry struct {
		Index int
		Level int
		Text  string
	}

	headings := make([]headingEntry, 0)
	for i, block := range blocks {
		level, text := notionHeadingLevelAndText(block)
		if level == 0 {
			continue
		}
		headings = append(headings, headingEntry{Index: i, Level: level, Text: text})
	}

	matches := make([]headingEntry, 0)
	for _, item := range headings {
		if item.Text != strings.TrimSpace(heading) {
			continue
		}
		if level > 0 && item.Level != level {
			continue
		}
		matches = append(matches, item)
	}

	if len(matches) == 0 {
		return BlockSection{}, fmt.Errorf("heading %q not found", heading)
	}
	if len(matches) > 1 {
		return BlockSection{}, fmt.Errorf("heading %q matched %d sections; use a unique heading or specify the level", heading, len(matches))
	}

	match := matches[0]
	end := len(blocks)
	for _, item := range headings {
		if item.Index <= match.Index {
			continue
		}
		if item.Level <= match.Level {
			end = item.Index
			break
		}
	}

	return BlockSection{
		Heading: match.Text,
		Level:   match.Level,
		Start:   match.Index,
		End:     end,
	}, nil
}

func notionHeadingLevelAndText(block notionBlock) (int, string) {
	switch block.Type {
	case "heading_1":
		if block.Heading1 != nil {
			return 1, joinRichText(block.Heading1.RichText)
		}
	case "heading_2":
		if block.Heading2 != nil {
			return 2, joinRichText(block.Heading2.RichText)
		}
	case "heading_3":
		if block.Heading3 != nil {
			return 3, joinRichText(block.Heading3.RichText)
		}
	}
	return 0, ""
}

func refuseSectionReplacementForProtectedBlocks(pageID string, blocks []notionBlock) error {
	for _, block := range blocks {
		if block.Type == "child_page" || block.Type == "child_database" {
			return fmt.Errorf("refusing section refresh for %s because the target section contains %s blocks", pageID, block.Type)
		}
	}
	return nil
}

func verifySectionBlocks(actual []notionBlock, expected []Block) error {
	actualSignatures := make([]string, 0, len(actual))
	for _, block := range actual {
		signature, ok := notionBlockSignature(block)
		if !ok {
			return fmt.Errorf("section verification encountered unsupported Notion block type %q", block.Type)
		}
		actualSignatures = append(actualSignatures, signature)
	}

	expectedSignatures := make([]string, 0, len(expected))
	for _, block := range expected {
		signature, ok := sourceBlockSignature(block)
		if !ok {
			return fmt.Errorf("section verification encountered unsupported source block kind %q", block.Kind)
		}
		expectedSignatures = append(expectedSignatures, signature)
	}

	if len(actualSignatures) != len(expectedSignatures) {
		return fmt.Errorf("notion section verification failed: expected %d blocks, got %d", len(expectedSignatures), len(actualSignatures))
	}
	for i := range expectedSignatures {
		if actualSignatures[i] == expectedSignatures[i] {
			continue
		}
		return fmt.Errorf("notion section verification failed at block %d: expected %q, got %q", i+1, expectedSignatures[i], actualSignatures[i])
	}
	return nil
}

func sourceBlockSignature(block Block) (string, bool) {
	switch block.Kind {
	case "paragraph", "heading_1", "heading_2", "heading_3", "bulleted_list_item", "numbered_list_item", "quote":
		return block.Kind + "\x00" + normalizeComparableText(renderSourceInlineText(block)), true
	case "to_do":
		return block.Kind + "\x00" + fmt.Sprintf("%t", block.Checked) + "\x00" + strings.TrimSpace(block.Color) + "\x00" + normalizeComparableText(renderSourceInlineText(block)), true
	case "toggle":
		return block.Kind + "\x00" + strings.TrimSpace(block.Color) + "\x00" + normalizeComparableText(renderSourceInlineText(block)) + "\x00" + blockChildrenSignature(block.Children), true
	case "divider":
		return block.Kind, true
	case "callout":
		return block.Kind + "\x00" + strings.TrimSpace(block.Icon) + "\x00" + strings.TrimSpace(block.Color) + "\x00" + normalizeComparableText(renderSourceInlineText(block)) + "\x00" + blockChildrenSignature(block.Children), true
	case "code":
		return block.Kind + "\x00" + strings.TrimSpace(block.CodeLanguage) + "\x00" + block.Text, true
	case "equation":
		return block.Kind + "\x00" + strings.TrimSpace(block.Text), true
	case "table":
		return block.Kind + "\x00" + fmt.Sprintf("%t", block.TableHeader) + "\x00" + fmt.Sprintf("%t", block.TableRowHead) + "\x00" + tableRowSignature(block.TableRows), true
	case "page_ref", "database_ref":
		return block.Kind + "\x00" + strings.TrimSpace(block.RefID), true
	case "image", "file", "pdf", "audio", "video":
		return block.Kind + "\x00" + strings.TrimSpace(block.Caption), true
	default:
		return "", false
	}
}

func notionBlockSignature(block notionBlock) (string, bool) {
	switch block.Type {
	case "paragraph":
		if block.Paragraph != nil {
			return block.Type + "\x00" + normalizeComparableText(joinRichText(block.Paragraph.RichText)), true
		}
	case "heading_1":
		if block.Heading1 != nil {
			return block.Type + "\x00" + normalizeComparableText(joinRichText(block.Heading1.RichText)), true
		}
	case "heading_2":
		if block.Heading2 != nil {
			return block.Type + "\x00" + normalizeComparableText(joinRichText(block.Heading2.RichText)), true
		}
	case "heading_3":
		if block.Heading3 != nil {
			return block.Type + "\x00" + normalizeComparableText(joinRichText(block.Heading3.RichText)), true
		}
	case "bulleted_list_item":
		if block.BulletedListItem != nil {
			return block.Type + "\x00" + normalizeComparableText(joinRichText(block.BulletedListItem.RichText)), true
		}
	case "numbered_list_item":
		if block.NumberedListItem != nil {
			return block.Type + "\x00" + normalizeComparableText(joinRichText(block.NumberedListItem.RichText)), true
		}
	case "to_do":
		if block.ToDo != nil {
			return block.Type + "\x00" + fmt.Sprintf("%t", block.ToDo.Checked) + "\x00" + strings.TrimSpace(block.ToDo.Color) + "\x00" + normalizeComparableText(joinRichText(block.ToDo.RichText)), true
		}
	case "toggle":
		if block.Toggle != nil {
			return block.Type + "\x00" + strings.TrimSpace(block.Toggle.Color) + "\x00" + normalizeComparableText(joinRichText(block.Toggle.RichText)) + "\x00" + notionChildrenSignature(block.Children), true
		}
	case "quote":
		if block.Quote != nil {
			return block.Type + "\x00" + normalizeComparableText(joinRichText(block.Quote.RichText)), true
		}
	case "divider":
		return block.Type, true
	case "callout":
		if block.Callout != nil {
			icon := ""
			if block.Callout.Icon != nil {
				icon = block.Callout.Icon.Emoji
			}
			return block.Type + "\x00" + strings.TrimSpace(icon) + "\x00" + strings.TrimSpace(block.Callout.Color) + "\x00" + normalizeComparableText(joinRichText(block.Callout.RichText)) + "\x00" + notionChildrenSignature(block.Children), true
		}
	case "code":
		if block.Code != nil {
			return block.Type + "\x00" + strings.TrimSpace(block.Code.Language) + "\x00" + joinRichText(block.Code.RichText), true
		}
	case "equation":
		if block.Equation != nil {
			return block.Type + "\x00" + strings.TrimSpace(block.Equation.Expression), true
		}
	case "table":
		if block.Table != nil {
			return block.Type + "\x00" + fmt.Sprintf("%t", block.Table.HasColumnHeader) + "\x00" + fmt.Sprintf("%t", block.Table.HasRowHeader) + "\x00" + notionTableChildrenSignature(block.Children), true
		}
	case "link_to_page":
		if block.LinkToPage != nil {
			if block.LinkToPage.Type == "page_id" {
				return "page_ref" + "\x00" + strings.TrimSpace(block.LinkToPage.PageID), true
			}
			if block.LinkToPage.Type == "database_id" {
				return "database_ref" + "\x00" + strings.TrimSpace(block.LinkToPage.DatabaseID), true
			}
		}
	case "image", "file", "pdf", "audio", "video":
		data := mediaBlockData(block)
		if data != nil {
			return block.Type + "\x00" + joinRichText(data.Caption), true
		}
	}
	return "", false
}

func blockChildrenSignature(children []Block) string {
	parts := make([]string, 0, len(children))
	for _, child := range children {
		if sig, ok := sourceBlockSignature(child); ok {
			parts = append(parts, sig)
		}
	}
	return strings.Join(parts, "\x1f")
}

func notionChildrenSignature(children []notionBlock) string {
	parts := make([]string, 0, len(children))
	for _, child := range children {
		if sig, ok := notionBlockSignature(child); ok {
			parts = append(parts, sig)
		}
	}
	return strings.Join(parts, "\x1f")
}

func tableRowSignature(rows [][][]Inline) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(row))
		for _, cell := range row {
			cells = append(cells, inlinesPlainText(cell))
		}
		lines = append(lines, strings.Join(cells, "\x1e"))
	}
	return strings.Join(lines, "\x1f")
}

func notionTableChildrenSignature(children []notionBlock) string {
	lines := make([]string, 0, len(children))
	for _, row := range children {
		if row.TableRow == nil {
			continue
		}
		cells := make([]string, 0, len(row.TableRow.Cells))
		for _, cell := range row.TableRow.Cells {
			cells = append(cells, joinRichText(cell))
		}
		lines = append(lines, strings.Join(cells, "\x1e"))
	}
	return strings.Join(lines, "\x1f")
}

func inlinesPlainText(inlines []Inline) string {
	var parts []string
	for _, inline := range inlines {
		parts = append(parts, inline.Text)
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func renderSourceInlineText(block Block) string {
	if len(block.Inlines) == 0 {
		return strings.TrimSpace(block.Text)
	}
	return normalizeComparableText(joinSourceInlines(block.Inlines))
}

func joinSourceInlines(inlines []Inline) string {
	var parts []string
	for _, inline := range inlines {
		text := inline.Text
		if strings.TrimSpace(inline.MentionType) != "" && strings.TrimSpace(inline.MentionID) != "" {
			switch inline.MentionType {
			case "page":
				label := strings.TrimSpace(text)
				if label == "" {
					label = strings.TrimSpace(inline.MentionID)
				}
				text = fmt.Sprintf(`<mention-page url="%s">%s</mention-page>`, notionPageURL(inline.MentionID), label)
			case "database":
				label := strings.TrimSpace(text)
				if label == "" {
					label = strings.TrimSpace(inline.MentionID)
				}
				text = fmt.Sprintf(`<mention-database url="%s">%s</mention-database>`, notionPageURL(inline.MentionID), label)
			case "user":
				label := strings.TrimSpace(text)
				if label == "" {
					label = strings.TrimSpace(inline.MentionID)
				}
				text = fmt.Sprintf(`<mention-user id="%s">%s</mention-user>`, strings.TrimSpace(inline.MentionID), label)
			default:
				text = "@" + inline.MentionType + "(" + strings.TrimSpace(inline.MentionID) + ")"
			}
		}
		if inline.Code {
			text = "`" + text + "`"
		} else {
			if inline.Bold {
				text = "**" + text + "**"
			}
			if inline.Italic {
				text = "*" + text + "*"
			}
			if inline.Strikethrough {
				text = "~~" + text + "~~"
			}
			if inline.Underline {
				text = `<span underline="true">` + text + `</span>`
			}
			if color := normalizeNotionColor(inline.Color); color != "" {
				text = `<span color="` + color + `">` + text + `</span>`
			}
		}
		if inline.URL != "" && !inline.Code {
			text = fmt.Sprintf("[%s](%s)", text, inline.URL)
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func normalizeComparableText(text string) string {
	text = strings.TrimSpace(text)
	return markdownLinkTrailingSlashRE.ReplaceAllString(text, "($1)")
}

func markdownDiffSummary(expected string, actual string) string {
	expectedLines := strings.Split(comparableMarkdown(expected), "\n")
	actualLines := strings.Split(comparableMarkdown(actual), "\n")
	limit := len(expectedLines)
	if len(actualLines) < limit {
		limit = len(actualLines)
	}
	for i := 0; i < limit; i++ {
		if expectedLines[i] == actualLines[i] {
			continue
		}
		return fmt.Sprintf("first mismatch at line %d: expected %q, got %q", i+1, truncateForError(expectedLines[i]), truncateForError(actualLines[i]))
	}
	if len(expectedLines) != len(actualLines) {
		return fmt.Sprintf("line count differs: expected %d, got %d", len(expectedLines), len(actualLines))
	}
	return "normalized markdown differs after the write"
}

func truncateForError(line string) string {
	line = strings.TrimSpace(line)
	if len(line) <= 120 {
		return line
	}
	return line[:117] + "..."
}
