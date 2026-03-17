package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type DatabaseDiscovery struct {
	DatabaseID    string              `json:"database_id"`
	DatabaseTitle string              `json:"database_title"`
	DatabaseURL   string              `json:"database_url"`
	DataSources   []DataSourceSummary `json:"data_sources"`
}

type DataSourceSummary struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ParentDatabaseID string `json:"parent_database_id,omitempty"`
}

type DataSourcePropertySummary struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Type string `json:"type"`
}

type DataSourceInfo struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	Description      string                      `json:"description,omitempty"`
	ParentDatabaseID string                      `json:"parent_database_id,omitempty"`
	DatabaseParentID string                      `json:"database_parent_id,omitempty"`
	DatabaseParent   string                      `json:"database_parent_type,omitempty"`
	InTrash          bool                        `json:"in_trash"`
	Properties       []DataSourcePropertySummary `json:"properties"`
}

type PagePropertySummary struct {
	Name  string `json:"name"`
	ID    string `json:"id"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type PageInfo struct {
	ID         string                `json:"id"`
	Title      string                `json:"title"`
	URL        string                `json:"url"`
	ParentType string                `json:"parent_type,omitempty"`
	ParentID   string                `json:"parent_id,omitempty"`
	InTrash    bool                  `json:"in_trash"`
	Properties []PagePropertySummary `json:"properties"`
}

type QueryEntrySummary struct {
	Object     string                `json:"object"`
	ID         string                `json:"id"`
	Title      string                `json:"title,omitempty"`
	URL        string                `json:"url,omitempty"`
	Properties []PagePropertySummary `json:"properties,omitempty"`
}

type DataSourceQueryOptions struct {
	Filter           any
	Sorts            []any
	StartCursor      string
	PageSize         int
	FilterProperties []string
	ResultType       string
}

type DataSourceQueryResult struct {
	DataSourceID   string              `json:"data_source_id"`
	DataSourceName string              `json:"data_source_name,omitempty"`
	Entries        []QueryEntrySummary `json:"entries"`
	HasMore        bool                `json:"has_more"`
	NextCursor     string              `json:"next_cursor,omitempty"`
}

type PagePropertyItemOptions struct {
	StartCursor string
	PageSize    int
}

type PagePropertyItemSummary struct {
	PageID     string `json:"page_id"`
	Property   string `json:"property"`
	PropertyID string `json:"property_id"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func (c *Client) RetrieveDatabase(ctx context.Context, databaseID string) (map[string]any, error) {
	var resp map[string]any
	if err := c.request(ctx, http.MethodGet, "/databases/"+strings.TrimSpace(databaseID), nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) RetrieveDataSource(ctx context.Context, dataSourceID string) (map[string]any, error) {
	var resp map[string]any
	if err := c.request(ctx, http.MethodGet, "/data_sources/"+strings.TrimSpace(dataSourceID), nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) UpdateDataSource(ctx context.Context, dataSourceID string, payload map[string]any) (map[string]any, error) {
	var resp map[string]any
	if err := c.request(ctx, http.MethodPatch, "/data_sources/"+strings.TrimSpace(dataSourceID), payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) QueryDataSource(ctx context.Context, dataSourceID string, options DataSourceQueryOptions) (map[string]any, error) {
	var resp map[string]any
	path := "/data_sources/" + strings.TrimSpace(dataSourceID) + "/query"
	queryValues := url.Values{}
	for _, item := range options.FilterProperties {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		queryValues.Add("filter_properties[]", item)
	}
	if encoded := queryValues.Encode(); encoded != "" {
		path += "?" + encoded
	}

	payload := map[string]any{}
	if options.Filter != nil {
		payload["filter"] = options.Filter
	}
	if len(options.Sorts) > 0 {
		payload["sorts"] = options.Sorts
	}
	if strings.TrimSpace(options.StartCursor) != "" {
		payload["start_cursor"] = strings.TrimSpace(options.StartCursor)
	}
	if options.PageSize > 0 {
		payload["page_size"] = options.PageSize
	}
	if strings.TrimSpace(options.ResultType) != "" {
		payload["result_type"] = strings.TrimSpace(options.ResultType)
	}

	var requestBody any
	if len(payload) > 0 {
		requestBody = payload
	}
	if err := c.request(ctx, http.MethodPost, path, requestBody, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) CreateRow(ctx context.Context, dataSourceID string, properties map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"parent": map[string]any{
			"type":           "data_source_id",
			"data_source_id": strings.TrimSpace(dataSourceID),
		},
		"properties": properties,
	}
	var resp map[string]any
	if err := c.request(ctx, http.MethodPost, "/pages", payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) UpdateRowProperties(ctx context.Context, pageID string, properties map[string]any) (map[string]any, error) {
	payload := map[string]any{"properties": properties}
	var resp map[string]any
	if err := c.request(ctx, http.MethodPatch, "/pages/"+strings.TrimSpace(pageID), payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) RetrievePage(ctx context.Context, pageID string) (map[string]any, error) {
	var resp map[string]any
	if err := c.request(ctx, http.MethodGet, "/pages/"+strings.TrimSpace(pageID), nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) RetrievePagePropertyItem(ctx context.Context, pageID string, property string, options PagePropertyItemOptions) (map[string]any, string, error) {
	propertyID, err := c.resolvePagePropertyID(ctx, pageID, property)
	if err != nil {
		return nil, "", err
	}
	decodedPropertyID, err := url.PathUnescape(propertyID)
	if err != nil {
		decodedPropertyID = propertyID
	}
	var resp map[string]any
	path := "/pages/" + strings.TrimSpace(pageID) + "/properties/" + url.PathEscape(decodedPropertyID)
	query := url.Values{}
	if strings.TrimSpace(options.StartCursor) != "" {
		query.Set("start_cursor", strings.TrimSpace(options.StartCursor))
	}
	if options.PageSize > 0 {
		query.Set("page_size", fmt.Sprintf("%d", options.PageSize))
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, "", err
	}
	return resp, propertyID, nil
}

func (c *Client) resolvePagePropertyID(ctx context.Context, pageID string, property string) (string, error) {
	page, err := c.RetrievePage(ctx, pageID)
	if err != nil {
		return "", err
	}
	properties, _ := page["properties"].(map[string]any)
	property = strings.TrimSpace(property)
	for name, raw := range properties {
		if name != property {
			continue
		}
		if propMap, ok := raw.(map[string]any); ok {
			if id, _ := propMap["id"].(string); strings.TrimSpace(id) != "" {
				return strings.TrimSpace(id), nil
			}
		}
	}
	if property == "" {
		return "", fmt.Errorf("property is required")
	}
	return property, nil
}

func (c *Client) DiscoverDataSourcesFromPage(ctx context.Context, pageID string, maxDepth int) ([]DatabaseDiscovery, error) {
	seen := map[string]bool{}
	discovered := []DatabaseDiscovery{}
	if err := c.discoverDataSourcesFromPage(ctx, strings.TrimSpace(pageID), maxDepth, seen, &discovered); err != nil {
		return nil, err
	}
	sort.Slice(discovered, func(i, j int) bool {
		return discovered[i].DatabaseTitle < discovered[j].DatabaseTitle
	})
	return discovered, nil
}

func (c *Client) discoverDataSourcesFromPage(ctx context.Context, pageID string, depth int, seen map[string]bool, out *[]DatabaseDiscovery) error {
	blocks, err := c.listBlockChildren(ctx, pageID)
	if err != nil {
		return err
	}

	for _, block := range blocks {
		switch block.Type {
		case "child_database":
			if seen[block.ID] {
				continue
			}
			seen[block.ID] = true
			database, err := c.RetrieveDatabase(ctx, block.ID)
			if err != nil {
				return err
			}
			*out = append(*out, SummarizeDatabase(database))
		case "child_page":
			if depth == 0 {
				continue
			}
			nextDepth := depth - 1
			if depth < 0 {
				nextDepth = depth
			}
			if err := c.discoverDataSourcesFromPage(ctx, block.ID, nextDepth, seen, out); err != nil {
				return err
			}
		}
	}
	return nil
}

func SummarizeDatabase(database map[string]any) DatabaseDiscovery {
	return DatabaseDiscovery{
		DatabaseID:    stringValue(database["id"]),
		DatabaseTitle: richTextPlain(database["title"]),
		DatabaseURL:   firstNonEmpty(stringValue(database["url"]), notionPageURL(stringValue(database["id"]))),
		DataSources:   extractDataSourceSummaries(database),
	}
}

func SummarizeDataSource(dataSource map[string]any) DataSourceInfo {
	props := make([]DataSourcePropertySummary, 0)
	if rawProps, ok := dataSource["properties"].(map[string]any); ok {
		names := make([]string, 0, len(rawProps))
		for name := range rawProps {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			propMap, _ := rawProps[name].(map[string]any)
			props = append(props, DataSourcePropertySummary{
				Name: name,
				ID:   stringValue(propMap["id"]),
				Type: stringValue(propMap["type"]),
			})
		}
	}

	parentDatabaseID := ""
	if parent, ok := dataSource["parent"].(map[string]any); ok {
		parentDatabaseID = stringValue(parent["database_id"])
	}

	databaseParentType := ""
	databaseParentID := ""
	if parent, ok := dataSource["database_parent"].(map[string]any); ok {
		databaseParentType = stringValue(parent["type"])
		switch databaseParentType {
		case "page_id":
			databaseParentID = stringValue(parent["page_id"])
		case "database_id":
			databaseParentID = stringValue(parent["database_id"])
		}
	}

	return DataSourceInfo{
		ID:               stringValue(dataSource["id"]),
		Name:             richTextPlain(dataSource["title"]),
		Description:      richTextPlain(dataSource["description"]),
		ParentDatabaseID: parentDatabaseID,
		DatabaseParentID: databaseParentID,
		DatabaseParent:   databaseParentType,
		InTrash:          boolValue(dataSource["in_trash"]),
		Properties:       props,
	}
}

func SummarizePage(page map[string]any) PageInfo {
	properties := summarizePageProperties(page)
	parentType := ""
	parentID := ""
	if parent, ok := page["parent"].(map[string]any); ok {
		parentType = stringValue(parent["type"])
		switch parentType {
		case "page_id":
			parentID = stringValue(parent["page_id"])
		case "database_id":
			parentID = stringValue(parent["database_id"])
		case "data_source_id":
			parentID = stringValue(parent["data_source_id"])
		}
	}
	return PageInfo{
		ID:         stringValue(page["id"]),
		Title:      pageTitleFromRaw(page),
		URL:        stringValue(page["url"]),
		ParentType: parentType,
		ParentID:   parentID,
		InTrash:    boolValue(page["in_trash"]),
		Properties: properties,
	}
}

func SummarizeQueryResult(dataSource map[string]any, queryResponse map[string]any) DataSourceQueryResult {
	entries := []QueryEntrySummary{}
	if rawResults, ok := queryResponse["results"].([]any); ok {
		for _, raw := range rawResults {
			item, _ := raw.(map[string]any)
			entries = append(entries, summarizeQueryEntry(item))
		}
	}
	return DataSourceQueryResult{
		DataSourceID:   stringValue(dataSource["id"]),
		DataSourceName: richTextPlain(dataSource["title"]),
		Entries:        entries,
		HasMore:        boolValue(queryResponse["has_more"]),
		NextCursor:     stringValue(queryResponse["next_cursor"]),
	}
}

func SummarizePagePropertyItem(pageID string, property string, propertyID string, item map[string]any) PagePropertyItemSummary {
	return PagePropertyItemSummary{
		PageID:     strings.TrimSpace(pageID),
		Property:   strings.TrimSpace(property),
		PropertyID: strings.TrimSpace(propertyID),
		Type:       stringValue(item["type"]),
		Value:      propertyItemDisplay(item),
		HasMore:    boolValue(item["has_more"]),
		NextCursor: stringValue(item["next_cursor"]),
	}
}

func extractDataSourceSummaries(database map[string]any) []DataSourceSummary {
	items := []DataSourceSummary{}
	rawSources, _ := database["data_sources"].([]any)
	for _, raw := range rawSources {
		item, _ := raw.(map[string]any)
		items = append(items, DataSourceSummary{
			ID:               stringValue(item["id"]),
			Name:             firstNonEmpty(stringValue(item["name"]), richTextPlain(item["title"])),
			ParentDatabaseID: stringValue(database["id"]),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func summarizeQueryEntry(item map[string]any) QueryEntrySummary {
	objectType := stringValue(item["object"])
	if objectType == "" {
		objectType = "page"
	}
	switch objectType {
	case "page":
		page := SummarizePage(item)
		return QueryEntrySummary{
			Object:     "page",
			ID:         page.ID,
			Title:      page.Title,
			URL:        page.URL,
			Properties: page.Properties,
		}
	case "data_source":
		ds := SummarizeDataSource(item)
		return QueryEntrySummary{
			Object: "data_source",
			ID:     ds.ID,
			Title:  ds.Name,
		}
	case "database":
		db := SummarizeDatabase(item)
		return QueryEntrySummary{
			Object: "database",
			ID:     db.DatabaseID,
			Title:  db.DatabaseTitle,
			URL:    db.DatabaseURL,
		}
	default:
		return QueryEntrySummary{
			Object: objectType,
			ID:     stringValue(item["id"]),
			Title:  firstNonEmpty(pageTitleFromRaw(item), richTextPlain(item["title"])),
			URL:    stringValue(item["url"]),
		}
	}
}

func summarizePageProperties(page map[string]any) []PagePropertySummary {
	rawProps, _ := page["properties"].(map[string]any)
	names := make([]string, 0, len(rawProps))
	for name := range rawProps {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]PagePropertySummary, 0, len(rawProps))
	for _, name := range names {
		propMap, _ := rawProps[name].(map[string]any)
		out = append(out, PagePropertySummary{
			Name:  name,
			ID:    stringValue(propMap["id"]),
			Type:  stringValue(propMap["type"]),
			Value: propertyValueDisplay(propMap),
		})
	}
	return out
}

func pageTitleFromRaw(page map[string]any) string {
	rawProps, _ := page["properties"].(map[string]any)
	for _, raw := range rawProps {
		propMap, _ := raw.(map[string]any)
		if stringValue(propMap["type"]) != "title" {
			continue
		}
		return richTextPlain(propMap["title"])
	}
	return ""
}

func propertyItemDisplay(item map[string]any) string {
	propType := stringValue(item["type"])
	if propType == "" {
		if results, ok := item["results"].([]any); ok {
			parts := make([]string, 0, len(results))
			for _, raw := range results {
				value, _ := raw.(map[string]any)
				display := propertyValueDisplay(value)
				if display == "" {
					display = stringValue(value["plain_text"])
				}
				if display != "" {
					parts = append(parts, display)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, ", ")
			}
			return fmt.Sprintf("%d paginated values", len(results))
		}
		return ""
	}
	return propertyValueDisplay(item)
}

func propertyValueDisplay(propMap map[string]any) string {
	propType := stringValue(propMap["type"])
	switch propType {
	case "title", "rich_text":
		return richTextPlain(propMap[propType])
	case "status", "select":
		item, _ := propMap[propType].(map[string]any)
		return stringValue(item["name"])
	case "multi_select":
		items, _ := propMap[propType].([]any)
		parts := make([]string, 0, len(items))
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			parts = append(parts, stringValue(item["name"]))
		}
		return strings.Join(parts, ", ")
	case "checkbox":
		if boolValue(propMap[propType]) {
			return "true"
		}
		return "false"
	case "number":
		return fmt.Sprintf("%v", propMap[propType])
	case "url", "email", "phone_number":
		return stringValue(propMap[propType])
	case "date":
		item, _ := propMap[propType].(map[string]any)
		start := stringValue(item["start"])
		end := stringValue(item["end"])
		if end != "" {
			return start + " -> " + end
		}
		return start
	case "relation":
		items, _ := propMap[propType].([]any)
		parts := make([]string, 0, len(items))
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			parts = append(parts, stringValue(item["id"]))
		}
		return strings.Join(parts, ", ")
	case "people":
		items, _ := propMap[propType].([]any)
		parts := make([]string, 0, len(items))
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			name := firstNonEmpty(stringValue(item["name"]), stringValue(item["id"]))
			parts = append(parts, name)
		}
		return strings.Join(parts, ", ")
	case "formula":
		item, _ := propMap[propType].(map[string]any)
		return propertyValueDisplay(item)
	case "created_time", "last_edited_time":
		return stringValue(propMap[propType])
	case "created_by", "last_edited_by":
		item, _ := propMap[propType].(map[string]any)
		return firstNonEmpty(stringValue(item["name"]), stringValue(item["id"]))
	case "unique_id":
		item, _ := propMap[propType].(map[string]any)
		prefix := stringValue(item["prefix"])
		number := fmt.Sprintf("%v", item["number"])
		return strings.TrimSpace(prefix + "-" + number)
	case "files":
		items, _ := propMap[propType].([]any)
		parts := make([]string, 0, len(items))
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			name := stringValue(item["name"])
			if name == "" {
				name = stringValue(item["type"])
			}
			parts = append(parts, name)
		}
		return strings.Join(parts, ", ")
	default:
		if nested, ok := propMap[propType].(map[string]any); ok {
			if name := stringValue(nested["name"]); name != "" {
				return name
			}
			if id := stringValue(nested["id"]); id != "" {
				return id
			}
		}
		if value, ok := propMap[propType].(string); ok {
			return value
		}
		return ""
	}
}

func richTextPlain(raw any) string {
	items, _ := raw.([]any)
	if len(items) == 0 {
		if typed, ok := raw.([]richTextItem); ok {
			return joinRichText(typed)
		}
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		plain := stringValue(item["plain_text"])
		if plain == "" {
			if text, ok := item["text"].(map[string]any); ok {
				plain = stringValue(text["content"])
			}
		}
		if plain == "" {
			if mention, ok := item["mention"].(map[string]any); ok {
				plain = firstNonEmpty(stringValue(mention["type"]), stringValue(item["href"]))
			}
		}
		parts = append(parts, plain)
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func stringValue(raw any) string {
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func boolValue(raw any) bool {
	if value, ok := raw.(bool); ok {
		return value
	}
	return false
}
