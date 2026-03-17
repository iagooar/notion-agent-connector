package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"notion-agent-connector/internal/config"
	"notion-agent-connector/internal/notion"
)

func runGetDatabase(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("get-database", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		databaseID string
		format     string
	)
	fs.StringVar(&databaseID, "database-id", "", "database id to retrieve")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(databaseID) == "" {
		return errors.New("-database-id is required")
	}

	database, err := client.RetrieveDatabase(ctx, strings.TrimSpace(databaseID))
	if err != nil {
		return err
	}
	return renderDatabaseSummary(notion.SummarizeDatabase(database), format)
}

func runListDataSources(ctx context.Context, client *notion.Client, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("list-data-sources", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		pageID     string
		databaseID string
		maxDepth   int
		format     string
	)
	fs.StringVar(&pageID, "page-id", "", "page id to scan for child databases")
	fs.StringVar(&databaseID, "database-id", "", "database id to inspect directly")
	fs.IntVar(&maxDepth, "max-depth", 2, "maximum child-page recursion depth when scanning from a page")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(pageID) != "" && strings.TrimSpace(databaseID) != "" {
		return errors.New("use either -page-id or -database-id, not both")
	}

	if strings.TrimSpace(databaseID) != "" {
		database, err := client.RetrieveDatabase(ctx, strings.TrimSpace(databaseID))
		if err != nil {
			return err
		}
		return renderDataSourceDiscoveries([]notion.DatabaseDiscovery{notion.SummarizeDatabase(database)}, format)
	}

	pageID = firstNonEmpty(pageID, cfg.ReadRootPageID)
	if strings.TrimSpace(pageID) == "" {
		return errors.New("NOTION_AGENT_CONNECTOR_READ_ROOT_PAGE_ID or -page-id is required")
	}
	discoveries, err := client.DiscoverDataSourcesFromPage(ctx, pageID, maxDepth)
	if err != nil {
		return err
	}
	return renderDataSourceDiscoveries(discoveries, format)
}

func runGetDataSource(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("get-data-source", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		dataSourceID string
		format       string
	)
	fs.StringVar(&dataSourceID, "data-source-id", "", "data source id to retrieve")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(dataSourceID) == "" {
		return errors.New("-data-source-id is required")
	}

	dataSource, err := client.RetrieveDataSource(ctx, strings.TrimSpace(dataSourceID))
	if err != nil {
		return err
	}
	return renderDataSourceInfo(notion.SummarizeDataSource(dataSource), format)
}

func runQueryDataSource(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("query-data-source", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		dataSourceID     string
		filterJSON       string
		sortJSON         string
		filterProperties string
		titleContains    string
		startCursor      string
		resultType       string
		pageSize         int
		format           string
	)
	fs.StringVar(&dataSourceID, "data-source-id", "", "data source id to query")
	fs.StringVar(&filterJSON, "filter-json", "", "filter JSON object or @path to JSON")
	fs.StringVar(&sortJSON, "sort-json", "", "sort JSON object/array or @path to JSON")
	fs.StringVar(&filterProperties, "filter-properties", "", "comma-separated property names or ids to request explicitly")
	fs.StringVar(&titleContains, "title-contains", "", "title search shorthand built on the data source title property")
	fs.StringVar(&startCursor, "cursor", "", "query start cursor")
	fs.StringVar(&resultType, "result-type", "", "optional result type")
	fs.IntVar(&pageSize, "page-size", 0, "optional page size")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(dataSourceID) == "" {
		return errors.New("-data-source-id is required")
	}
	if strings.TrimSpace(filterJSON) != "" && strings.TrimSpace(titleContains) != "" {
		return errors.New("use either -filter-json or -title-contains, not both")
	}

	dataSource, err := client.RetrieveDataSource(ctx, strings.TrimSpace(dataSourceID))
	if err != nil {
		return err
	}

	var filter any
	if strings.TrimSpace(filterJSON) != "" {
		filter, err = loadJSONValue(filterJSON)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(titleContains) != "" {
		titleProperty, err := titlePropertyName(notion.SummarizeDataSource(dataSource))
		if err != nil {
			return err
		}
		filter = map[string]any{
			"property": titleProperty,
			"title": map[string]any{
				"contains": strings.TrimSpace(titleContains),
			},
		}
	}

	sorts, err := loadJSONArrayValue(sortJSON)
	if err != nil {
		return err
	}

	query, err := client.QueryDataSource(ctx, strings.TrimSpace(dataSourceID), notion.DataSourceQueryOptions{
		Filter:           filter,
		Sorts:            sorts,
		StartCursor:      strings.TrimSpace(startCursor),
		PageSize:         pageSize,
		FilterProperties: splitCommaList(filterProperties),
		ResultType:       strings.TrimSpace(resultType),
	})
	if err != nil {
		return err
	}
	return renderDataSourceQueryResult(notion.SummarizeQueryResult(dataSource, query), format)
}

func runCreateRow(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("create-row", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		dataSourceID  string
		propertiesArg string
		format        string
	)
	fs.StringVar(&dataSourceID, "data-source-id", "", "data source id that should own the new row")
	fs.StringVar(&propertiesArg, "properties-json", "", "properties JSON object or @path to JSON")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(dataSourceID) == "" {
		return errors.New("-data-source-id is required")
	}
	properties, err := loadJSONObjectValue(propertiesArg)
	if err != nil {
		return err
	}
	row, err := client.CreateRow(ctx, strings.TrimSpace(dataSourceID), properties)
	if err != nil {
		return err
	}
	return renderPageInfo(notion.SummarizePage(row), format)
}

func runGetRow(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("get-row", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		pageID string
		format string
	)
	fs.StringVar(&pageID, "page-id", "", "row page id to retrieve")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(pageID) == "" {
		return errors.New("-page-id is required")
	}
	row, err := client.RetrievePage(ctx, strings.TrimSpace(pageID))
	if err != nil {
		return err
	}
	return renderPageInfo(notion.SummarizePage(row), format)
}

func runUpdateRowProperties(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("update-row-properties", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		pageID        string
		propertiesArg string
		format        string
	)
	fs.StringVar(&pageID, "page-id", "", "row page id to update")
	fs.StringVar(&propertiesArg, "properties-json", "", "properties JSON object or @path to JSON")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(pageID) == "" {
		return errors.New("-page-id is required")
	}
	properties, err := loadJSONObjectValue(propertiesArg)
	if err != nil {
		return err
	}
	row, err := client.UpdateRowProperties(ctx, strings.TrimSpace(pageID), properties)
	if err != nil {
		return err
	}
	return renderPageInfo(notion.SummarizePage(row), format)
}

func runGetRowProperty(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("get-row-property", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		pageID   string
		property string
		cursor   string
		pageSize int
		format   string
	)
	fs.StringVar(&pageID, "page-id", "", "row page id to inspect")
	fs.StringVar(&property, "property", "", "property name or property id")
	fs.StringVar(&cursor, "cursor", "", "optional property-item pagination cursor")
	fs.IntVar(&pageSize, "page-size", 0, "optional property-item page size")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(pageID) == "" {
		return errors.New("-page-id is required")
	}
	if strings.TrimSpace(property) == "" {
		return errors.New("-property is required")
	}

	item, propertyID, err := client.RetrievePagePropertyItem(ctx, strings.TrimSpace(pageID), strings.TrimSpace(property), notion.PagePropertyItemOptions{
		StartCursor: strings.TrimSpace(cursor),
		PageSize:    pageSize,
	})
	if err != nil {
		return err
	}
	return renderPagePropertyItem(notion.SummarizePagePropertyItem(strings.TrimSpace(pageID), strings.TrimSpace(property), propertyID, item), format)
}

func runUpdateDataSource(ctx context.Context, client *notion.Client, args []string) error {
	fs := flag.NewFlagSet("update-data-source", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		dataSourceID  string
		title         string
		description   string
		propertiesArg string
		format        string
	)
	fs.StringVar(&dataSourceID, "data-source-id", "", "data source id to update")
	fs.StringVar(&title, "title", "", "optional plain-text data source title")
	fs.StringVar(&description, "description", "", "optional plain-text description")
	fs.StringVar(&propertiesArg, "properties-json", "", "optional properties JSON object or @path to JSON")
	fs.StringVar(&format, "format", "json", "output format: json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(dataSourceID) == "" {
		return errors.New("-data-source-id is required")
	}

	payload := map[string]any{}
	if strings.TrimSpace(title) != "" {
		payload["title"] = []any{richTextInput(strings.TrimSpace(title))}
	}
	if strings.TrimSpace(description) != "" {
		payload["description"] = []any{richTextInput(strings.TrimSpace(description))}
	}
	if strings.TrimSpace(propertiesArg) != "" {
		properties, err := loadJSONObjectValue(propertiesArg)
		if err != nil {
			return err
		}
		payload["properties"] = properties
	}
	if len(payload) == 0 {
		return errors.New("at least one of -title, -description, or -properties-json is required")
	}

	updated, err := client.UpdateDataSource(ctx, strings.TrimSpace(dataSourceID), payload)
	if err != nil {
		return err
	}
	return renderDataSourceInfo(notion.SummarizeDataSource(updated), format)
}

func renderDatabaseSummary(summary notion.DatabaseDiscovery, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(summary)
	case "markdown", "md":
		fmt.Printf("# %s\n\n", summary.DatabaseTitle)
		fmt.Printf("- Database ID: %s\n", summary.DatabaseID)
		fmt.Printf("- URL: %s\n", summary.DatabaseURL)
		fmt.Printf("- Data sources: %d\n", len(summary.DataSources))
		for _, item := range summary.DataSources {
			fmt.Printf("- %s: %s\n", item.Name, item.ID)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderDataSourceDiscoveries(discoveries []notion.DatabaseDiscovery, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(discoveries)
	case "markdown", "md":
		fmt.Println("# Data Sources")
		fmt.Println("")
		if len(discoveries) == 0 {
			fmt.Println("No child databases or data sources found.")
			return nil
		}
		for _, discovery := range discoveries {
			fmt.Printf("## %s\n\n", discovery.DatabaseTitle)
			fmt.Printf("- Database ID: %s\n", discovery.DatabaseID)
			fmt.Printf("- URL: %s\n", discovery.DatabaseURL)
			for _, item := range discovery.DataSources {
				fmt.Printf("- Data source: %s (%s)\n", item.Name, item.ID)
			}
			fmt.Println("")
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderDataSourceInfo(info notion.DataSourceInfo, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(info)
	case "markdown", "md":
		fmt.Printf("# %s\n\n", info.Name)
		fmt.Printf("- Data source ID: %s\n", info.ID)
		if info.ParentDatabaseID != "" {
			fmt.Printf("- Parent database ID: %s\n", info.ParentDatabaseID)
		}
		if info.DatabaseParentID != "" {
			fmt.Printf("- Database parent %s: %s\n", info.DatabaseParent, info.DatabaseParentID)
		}
		fmt.Printf("- In trash: %t\n", info.InTrash)
		if info.Description != "" {
			fmt.Printf("- Description: %s\n", info.Description)
		}
		fmt.Println("")
		fmt.Println("## Properties")
		fmt.Println("")
		for _, prop := range info.Properties {
			fmt.Printf("- %s [%s]: %s\n", prop.Name, prop.Type, prop.ID)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderDataSourceQueryResult(result notion.DataSourceQueryResult, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(result)
	case "markdown", "md":
		fmt.Printf("# %s\n\n", firstNonEmpty(result.DataSourceName, "Data source query"))
		fmt.Printf("- Data source ID: %s\n", result.DataSourceID)
		fmt.Printf("- Entries: %d\n", len(result.Entries))
		fmt.Printf("- Has more: %t\n", result.HasMore)
		if result.NextCursor != "" {
			fmt.Printf("- Next cursor: %s\n", result.NextCursor)
		}
		fmt.Println("")
		for _, entry := range result.Entries {
			fmt.Printf("## %s\n\n", firstNonEmpty(entry.Title, entry.ID))
			fmt.Printf("- Object: %s\n", entry.Object)
			fmt.Printf("- ID: %s\n", entry.ID)
			if entry.URL != "" {
				fmt.Printf("- URL: %s\n", entry.URL)
			}
			for _, prop := range entry.Properties {
				fmt.Printf("- %s [%s]: %s\n", prop.Name, prop.Type, prop.Value)
			}
			fmt.Println("")
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderPageInfo(info notion.PageInfo, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(info)
	case "markdown", "md":
		fmt.Printf("# %s\n\n", firstNonEmpty(info.Title, info.ID))
		fmt.Printf("- Page ID: %s\n", info.ID)
		fmt.Printf("- URL: %s\n", info.URL)
		if info.ParentID != "" {
			fmt.Printf("- Parent %s: %s\n", info.ParentType, info.ParentID)
		}
		fmt.Printf("- In trash: %t\n", info.InTrash)
		fmt.Println("")
		fmt.Println("## Properties")
		fmt.Println("")
		for _, prop := range info.Properties {
			fmt.Printf("- %s [%s]: %s\n", prop.Name, prop.Type, prop.Value)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderPagePropertyItem(item notion.PagePropertyItemSummary, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(item)
	case "markdown", "md":
		fmt.Printf("# %s\n\n", item.Property)
		fmt.Printf("- Page ID: %s\n", item.PageID)
		fmt.Printf("- Property ID: %s\n", item.PropertyID)
		fmt.Printf("- Type: %s\n", item.Type)
		fmt.Printf("- Value: %s\n", item.Value)
		fmt.Printf("- Has more: %t\n", item.HasMore)
		if item.NextCursor != "" {
			fmt.Printf("- Next cursor: %s\n", item.NextCursor)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func richTextInput(text string) map[string]any {
	return map[string]any{
		"type": "text",
		"text": map[string]any{
			"content": text,
		},
	}
}

func loadJSONObjectValue(value string) (map[string]any, error) {
	raw, err := loadJSONSource(value)
	if err != nil {
		return nil, err
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("expected a JSON object")
	}
	return obj, nil
}

func loadJSONArrayValue(value string) ([]any, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := loadJSONSource(value)
	if err != nil {
		return nil, err
	}
	if arr, ok := raw.([]any); ok {
		return arr, nil
	}
	if obj, ok := raw.(map[string]any); ok {
		return []any{obj}, nil
	}
	return nil, errors.New("expected a JSON array or object")
}

func loadJSONValue(value string) (any, error) {
	return loadJSONSource(value)
}

func loadJSONSource(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("JSON input is required")
	}
	if strings.HasPrefix(value, "@") {
		raw, err := os.ReadFile(strings.TrimPrefix(value, "@"))
		if err != nil {
			return nil, err
		}
		value = string(raw)
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func titlePropertyName(info notion.DataSourceInfo) (string, error) {
	for _, prop := range info.Properties {
		if prop.Type == "title" {
			return prop.Name, nil
		}
	}
	return "", errors.New("data source has no title property")
}
