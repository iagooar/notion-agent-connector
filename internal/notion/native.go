package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type BlockChildrenPage struct {
	Results    []map[string]any `json:"results"`
	HasMore    bool             `json:"has_more"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type AppendPosition struct {
	Kind         string
	AfterBlockID string
}

type SearchOptions struct {
	Query       string
	Filter      any
	Sort        any
	StartCursor string
	PageSize    int
}

type SearchResult struct {
	Results    []map[string]any `json:"results"`
	HasMore    bool             `json:"has_more"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type SearchEntrySummary struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`
}

type SearchResultSummary struct {
	Query      string               `json:"query,omitempty"`
	Scope      string               `json:"scope"`
	Entries    []SearchEntrySummary `json:"entries"`
	HasMore    bool                 `json:"has_more"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

type RootSearchOptions struct {
	Query            string
	MaxDepth         int
	IncludeRowTitles bool
}

type CommentListResult struct {
	Results    []map[string]any `json:"results"`
	HasMore    bool             `json:"has_more"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type CommentCreateInput struct {
	PageID       string
	BlockID      string
	DiscussionID string
	RichText     []any
}

type CommentSummary struct {
	ID           string `json:"id"`
	DiscussionID string `json:"discussion_id,omitempty"`
	ParentType   string `json:"parent_type,omitempty"`
	ParentID     string `json:"parent_id,omitempty"`
	Text         string `json:"text"`
	CreatedBy    string `json:"created_by,omitempty"`
	CreatedTime  string `json:"created_time,omitempty"`
}

type CommentSummaryList struct {
	Comments   []CommentSummary `json:"comments"`
	HasMore    bool             `json:"has_more"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

func (c *Client) RetrieveBlockRaw(ctx context.Context, blockID string) (map[string]any, error) {
	var resp map[string]any
	if err := c.request(ctx, http.MethodGet, "/blocks/"+strings.TrimSpace(blockID), nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) ListBlockChildrenPageRaw(ctx context.Context, blockID string, startCursor string, pageSize int) (BlockChildrenPage, error) {
	path := fmt.Sprintf("/blocks/%s/children", strings.TrimSpace(blockID))
	query := url.Values{}
	if pageSize > 0 {
		query.Set("page_size", fmt.Sprintf("%d", pageSize))
	}
	if strings.TrimSpace(startCursor) != "" {
		query.Set("start_cursor", strings.TrimSpace(startCursor))
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var resp BlockChildrenPage
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return BlockChildrenPage{}, err
	}
	return resp, nil
}

func (c *Client) ListBlockChildrenRecursiveRaw(ctx context.Context, blockID string) ([]map[string]any, error) {
	blocks, err := c.listBlockChildren(ctx, strings.TrimSpace(blockID))
	if err != nil {
		return nil, err
	}
	return blocksToRaw(blocks)
}

func (c *Client) AppendBlockChildrenRaw(ctx context.Context, blockID string, children []map[string]any, position AppendPosition) (BlockChildrenPage, error) {
	payload := map[string]any{
		"children": children,
	}
	if positionPayload := position.payload(); positionPayload != nil {
		payload["position"] = positionPayload
	}
	var resp BlockChildrenPage
	if err := c.request(ctx, http.MethodPatch, "/blocks/"+strings.TrimSpace(blockID)+"/children", payload, &resp); err != nil {
		return BlockChildrenPage{}, err
	}
	return resp, nil
}

func (p AppendPosition) payload() map[string]any {
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

func (c *Client) UpdateBlockRaw(ctx context.Context, blockID string, payload map[string]any) (map[string]any, error) {
	var resp map[string]any
	if err := c.request(ctx, http.MethodPatch, "/blocks/"+strings.TrimSpace(blockID), payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) DeleteBlockRaw(ctx context.Context, blockID string) (map[string]any, error) {
	var resp map[string]any
	if err := c.request(ctx, http.MethodDelete, "/blocks/"+strings.TrimSpace(blockID), nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) SearchWorkspace(ctx context.Context, options SearchOptions) (SearchResult, error) {
	payload := map[string]any{}
	if strings.TrimSpace(options.Query) != "" {
		payload["query"] = strings.TrimSpace(options.Query)
	}
	if options.Filter != nil {
		payload["filter"] = options.Filter
	}
	if options.Sort != nil {
		payload["sort"] = options.Sort
	}
	if strings.TrimSpace(options.StartCursor) != "" {
		payload["start_cursor"] = strings.TrimSpace(options.StartCursor)
	}
	if options.PageSize > 0 {
		payload["page_size"] = options.PageSize
	}

	var requestBody any
	if len(payload) > 0 {
		requestBody = payload
	}

	var resp SearchResult
	if err := c.request(ctx, http.MethodPost, "/search", requestBody, &resp); err != nil {
		return SearchResult{}, err
	}
	return resp, nil
}

func (c *Client) SearchRoot(ctx context.Context, rootPageID string, options RootSearchOptions) (SearchResultSummary, error) {
	pageSummaries := map[string]SearchEntrySummary{}
	if err := c.collectRootSearchEntries(ctx, strings.TrimSpace(rootPageID), options.MaxDepth, pageSummaries); err != nil {
		return SearchResultSummary{}, err
	}

	query := strings.ToLower(strings.TrimSpace(options.Query))
	entries := make([]SearchEntrySummary, 0, len(pageSummaries))
	for _, entry := range pageSummaries {
		if query != "" && !strings.Contains(strings.ToLower(entry.Title), query) {
			continue
		}
		entries = append(entries, entry)
	}

	if options.IncludeRowTitles {
		discoveries, err := c.DiscoverDataSourcesFromPage(ctx, strings.TrimSpace(rootPageID), options.MaxDepth)
		if err != nil {
			return SearchResultSummary{}, err
		}
		for _, discovery := range discoveries {
			for _, source := range discovery.DataSources {
				queryResult, err := c.QueryDataSource(ctx, source.ID, DataSourceQueryOptions{
					Filter: map[string]any{
						"property": firstNonEmpty(titlePropertyNameFromRaw(ctx, c, source.ID), "Name"),
						"title": map[string]any{
							"contains": options.Query,
						},
					},
				})
				if err != nil {
					continue
				}
				for _, entry := range SummarizeQueryResult(map[string]any{"id": source.ID, "title": []any{map[string]any{"plain_text": source.Name}}}, queryResult).Entries {
					if strings.TrimSpace(entry.Title) == "" {
						continue
					}
					entries = append(entries, SearchEntrySummary{
						Object: entry.Object,
						ID:     entry.ID,
						Title:  entry.Title,
						URL:    entry.URL,
					})
				}
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Object == entries[j].Object {
			return entries[i].Title < entries[j].Title
		}
		return entries[i].Object < entries[j].Object
	})

	return SearchResultSummary{
		Query:   strings.TrimSpace(options.Query),
		Scope:   "root",
		Entries: entries,
	}, nil
}

func (c *Client) collectRootSearchEntries(ctx context.Context, pageID string, depth int, out map[string]SearchEntrySummary) error {
	if strings.TrimSpace(pageID) == "" {
		return nil
	}
	page, err := c.RetrievePage(ctx, pageID)
	if err != nil {
		return err
	}
	pageSummary := SummarizePage(page)
	out[pageSummary.ID] = SearchEntrySummary{
		Object: "page",
		ID:     pageSummary.ID,
		Title:  pageSummary.Title,
		URL:    pageSummary.URL,
	}
	if depth <= 0 {
		return nil
	}

	children, err := c.listBlockChildren(ctx, pageID)
	if err != nil {
		return err
	}
	for _, child := range children {
		switch child.Type {
		case "child_page":
			if err := c.collectRootSearchEntries(ctx, child.ID, depth-1, out); err != nil {
				return err
			}
		case "child_database":
			out[child.ID] = SearchEntrySummary{
				Object: "database",
				ID:     child.ID,
				Title:  firstNonEmpty(childDatabaseTitle(child), strings.TrimSpace(child.ID)),
				URL:    notionPageURL(child.ID),
			}
		}
	}
	return nil
}

func (c *Client) ListComments(ctx context.Context, blockID string, startCursor string, pageSize int) (CommentListResult, error) {
	path := "/comments"
	query := url.Values{}
	query.Set("block_id", strings.TrimSpace(blockID))
	if strings.TrimSpace(startCursor) != "" {
		query.Set("start_cursor", strings.TrimSpace(startCursor))
	}
	if pageSize > 0 {
		query.Set("page_size", fmt.Sprintf("%d", pageSize))
	}
	path += "?" + query.Encode()
	var resp CommentListResult
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return CommentListResult{}, err
	}
	return resp, nil
}

func (c *Client) CreateComment(ctx context.Context, input CommentCreateInput) (map[string]any, error) {
	payload := map[string]any{
		"rich_text": input.RichText,
	}
	switch {
	case strings.TrimSpace(input.DiscussionID) != "":
		payload["discussion_id"] = strings.TrimSpace(input.DiscussionID)
	case strings.TrimSpace(input.PageID) != "":
		payload["parent"] = map[string]any{
			"type":    "page_id",
			"page_id": strings.TrimSpace(input.PageID),
		}
	case strings.TrimSpace(input.BlockID) != "":
		payload["parent"] = map[string]any{
			"type":     "block_id",
			"block_id": strings.TrimSpace(input.BlockID),
		}
	default:
		return nil, fmt.Errorf("page id, block id, or discussion id is required")
	}
	var resp map[string]any
	if err := c.request(ctx, http.MethodPost, "/comments", payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func RichTextInputFromText(text string) []any {
	return richTextSegments(parseInline(text))
}

func RenderRawBlockMarkdown(raw map[string]any) string {
	block, err := rawToBlock(raw)
	if err != nil {
		return ""
	}
	lines := renderBlocksMarkdown([]notionBlock{block}, 0, "")
	return strings.TrimSpace(strings.Join(compactMarkdown(lines), "\n"))
}

func RenderRawBlocksMarkdown(raw []map[string]any) string {
	blocks, err := rawToBlocks(raw)
	if err != nil {
		return ""
	}
	lines := renderBlocksMarkdown(blocks, 0, "")
	return strings.TrimSpace(strings.Join(compactMarkdown(lines), "\n"))
}

func SummarizeSearchResult(scope string, query string, result SearchResult) SearchResultSummary {
	entries := make([]SearchEntrySummary, 0, len(result.Results))
	for _, raw := range result.Results {
		entries = append(entries, summarizeSearchEntry(raw))
	}
	return SearchResultSummary{
		Query:      strings.TrimSpace(query),
		Scope:      strings.TrimSpace(scope),
		Entries:    entries,
		HasMore:    result.HasMore,
		NextCursor: result.NextCursor,
	}
}

func SummarizeCommentList(result CommentListResult) CommentSummaryList {
	items := make([]CommentSummary, 0, len(result.Results))
	for _, raw := range result.Results {
		items = append(items, summarizeComment(raw))
	}
	return CommentSummaryList{
		Comments:   items,
		HasMore:    result.HasMore,
		NextCursor: result.NextCursor,
	}
}

func SummarizeComment(raw map[string]any) CommentSummary {
	return summarizeComment(raw)
}

func summarizeComment(raw map[string]any) CommentSummary {
	parentType := ""
	parentID := ""
	if parent, ok := raw["parent"].(map[string]any); ok {
		parentType = stringValue(parent["type"])
		switch parentType {
		case "page_id":
			parentID = stringValue(parent["page_id"])
		case "block_id":
			parentID = stringValue(parent["block_id"])
		}
	}
	createdBy := ""
	if person, ok := raw["created_by"].(map[string]any); ok {
		createdBy = firstNonEmpty(stringValue(person["name"]), stringValue(person["id"]))
	}
	return CommentSummary{
		ID:           stringValue(raw["id"]),
		DiscussionID: stringValue(raw["discussion_id"]),
		ParentType:   parentType,
		ParentID:     parentID,
		Text:         richTextPlain(raw["rich_text"]),
		CreatedBy:    createdBy,
		CreatedTime:  stringValue(raw["created_time"]),
	}
}

func summarizeSearchEntry(raw map[string]any) SearchEntrySummary {
	objectType := firstNonEmpty(stringValue(raw["object"]), stringValue(raw["type"]))
	switch objectType {
	case "page":
		page := SummarizePage(raw)
		return SearchEntrySummary{Object: "page", ID: page.ID, Title: page.Title, URL: page.URL}
	case "database":
		database := SummarizeDatabase(raw)
		return SearchEntrySummary{Object: "database", ID: database.DatabaseID, Title: database.DatabaseTitle, URL: database.DatabaseURL}
	case "data_source":
		source := SummarizeDataSource(raw)
		return SearchEntrySummary{Object: "data_source", ID: source.ID, Title: source.Name}
	default:
		return SearchEntrySummary{
			Object: objectType,
			ID:     stringValue(raw["id"]),
			Title:  firstNonEmpty(pageTitleFromRaw(raw), richTextPlain(raw["title"])),
			URL:    stringValue(raw["url"]),
		}
	}
}

func rawToBlock(raw map[string]any) (notionBlock, error) {
	var block notionBlock
	encoded, err := json.Marshal(raw)
	if err != nil {
		return notionBlock{}, err
	}
	if err := json.Unmarshal(encoded, &block); err != nil {
		return notionBlock{}, err
	}
	return block, nil
}

func rawToBlocks(raw []map[string]any) ([]notionBlock, error) {
	blocks := make([]notionBlock, 0, len(raw))
	for _, item := range raw {
		block, err := rawToBlock(item)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func blocksToRaw(blocks []notionBlock) ([]map[string]any, error) {
	encoded, err := json.Marshal(blocks)
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func titlePropertyNameFromRaw(ctx context.Context, client *Client, dataSourceID string) string {
	source, err := client.RetrieveDataSource(ctx, dataSourceID)
	if err != nil {
		return ""
	}
	for _, property := range SummarizeDataSource(source).Properties {
		if property.Type == "title" {
			return property.Name
		}
	}
	return ""
}

func childDatabaseTitle(block notionBlock) string {
	if block.ChildDatabase != nil {
		return strings.TrimSpace(block.ChildDatabase.Title)
	}
	return ""
}
