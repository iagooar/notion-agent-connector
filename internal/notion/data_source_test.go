package notion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDiscoverDataSourcesFromPageFindsChildDatabases(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/blocks/root-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"db-1","type":"child_database","child_database":{"title":"Ops DB"}},{"id":"child-page","type":"child_page","child_page":{"title":"Nested"}}],"has_more":false,"next_cursor":null}`))
	})
	mux.HandleFunc("/v1/blocks/child-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"db-2","type":"child_database","child_database":{"title":"Nested DB"}}],"has_more":false,"next_cursor":null}`))
	})
	mux.HandleFunc("/v1/databases/db-1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"db-1","title":[{"plain_text":"Ops DB"}],"url":"https://www.notion.so/db1","data_sources":[{"id":"ds-1","name":"Primary"}]}`))
	})
	mux.HandleFunc("/v1/databases/db-2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"db-2","title":[{"plain_text":"Nested DB"}],"url":"https://www.notion.so/db2","data_sources":[{"id":"ds-2","name":"Nested Source"}]}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	discoveries, err := client.DiscoverDataSourcesFromPage(context.Background(), "root-page", 1)
	if err != nil {
		t.Fatalf("DiscoverDataSourcesFromPage() error = %v", err)
	}
	if len(discoveries) != 2 {
		t.Fatalf("len(discoveries) = %d", len(discoveries))
	}
	if discoveries[0].DatabaseID != "db-2" || discoveries[1].DatabaseID != "db-1" {
		t.Fatalf("discoveries = %#v", discoveries)
	}
}

func TestQueryDataSourcePassesFiltersSortsAndFilterProperties(t *testing.T) {
	var (
		body        map[string]any
		queryValues url.Values
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/data_sources/ds-1/query", func(w http.ResponseWriter, r *http.Request) {
		queryValues = r.URL.Query()
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"results":[{"object":"page","id":"row-1","url":"https://www.notion.so/row1","properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Row 1"}]}}}],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err := client.QueryDataSource(context.Background(), "ds-1", DataSourceQueryOptions{
		Filter:      map[string]any{"property": "Name"},
		Sorts:       []any{map[string]any{"property": "Name", "direction": "ascending"}},
		PageSize:    20,
		StartCursor: "cursor-1",
		FilterProperties: []string{
			"Name",
			"Status",
		},
		ResultType: "page",
	})
	if err != nil {
		t.Fatalf("QueryDataSource() error = %v", err)
	}
	if queryValues.Get("filter_properties[]") == "" {
		t.Fatalf("filter_properties query missing: %#v", queryValues)
	}
	if body["page_size"] != float64(20) {
		t.Fatalf("page_size = %#v", body["page_size"])
	}
	if body["start_cursor"] != "cursor-1" {
		t.Fatalf("start_cursor = %#v", body["start_cursor"])
	}
	if body["result_type"] != "page" {
		t.Fatalf("result_type = %#v", body["result_type"])
	}
}

func TestCreateRowUsesDataSourceParent(t *testing.T) {
	var payload map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(`{"id":"row-1","url":"https://www.notion.so/row1","properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Row 1"}]}}}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err := client.CreateRow(context.Background(), "ds-1", map[string]any{
		"Name": map[string]any{"title": []any{map[string]any{"text": map[string]any{"content": "Row 1"}}}},
	})
	if err != nil {
		t.Fatalf("CreateRow() error = %v", err)
	}
	parent, _ := payload["parent"].(map[string]any)
	if parent["type"] != "data_source_id" || parent["data_source_id"] != "ds-1" {
		t.Fatalf("parent = %#v", parent)
	}
}

func TestUpdateRowPropertiesPatchesPageProperties(t *testing.T) {
	var payload map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/row-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("unexpected method %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(`{"id":"row-1","url":"https://www.notion.so/row1","properties":{"Status":{"id":"status","type":"status","status":{"name":"Done"}}}}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err := client.UpdateRowProperties(context.Background(), "row-1", map[string]any{
		"Status": map[string]any{"status": map[string]any{"name": "Done"}},
	})
	if err != nil {
		t.Fatalf("UpdateRowProperties() error = %v", err)
	}
	properties, _ := payload["properties"].(map[string]any)
	if _, ok := properties["Status"]; !ok {
		t.Fatalf("properties = %#v", properties)
	}
}

func TestRetrievePagePropertyItemResolvesPropertyNameToID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/row-1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"row-1","properties":{"Status":{"id":"st_%3A","type":"status","status":{"name":"Todo"}}}}`))
	})
	mux.HandleFunc("/v1/pages/row-1/properties/st_%3A", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"object":"property_item","id":"st_%3A","type":"status","status":{"name":"Todo"}}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	item, propertyID, err := client.RetrievePagePropertyItem(context.Background(), "row-1", "Status", PagePropertyItemOptions{})
	if err != nil {
		t.Fatalf("RetrievePagePropertyItem() error = %v", err)
	}
	if propertyID != "st_%3A" {
		t.Fatalf("propertyID = %q", propertyID)
	}
	if item["type"] != "status" {
		t.Fatalf("item = %#v", item)
	}
}

func TestSummarizeQueryResult(t *testing.T) {
	dataSource := map[string]any{"id": "ds-1", "title": []any{map[string]any{"plain_text": "Ops Data"}}}
	queryResponse := map[string]any{
		"results": []any{
			map[string]any{
				"object": "page",
				"id":     "row-1",
				"url":    "https://www.notion.so/row1",
				"properties": map[string]any{
					"Name": map[string]any{"id": "title", "type": "title", "title": []any{map[string]any{"plain_text": "Row 1"}}},
				},
			},
		},
		"has_more":    true,
		"next_cursor": "cursor-2",
	}

	summary := SummarizeQueryResult(dataSource, queryResponse)
	if summary.DataSourceName != "Ops Data" {
		t.Fatalf("DataSourceName = %q", summary.DataSourceName)
	}
	if len(summary.Entries) != 1 || summary.Entries[0].Title != "Row 1" {
		t.Fatalf("Entries = %#v", summary.Entries)
	}
	if !summary.HasMore || summary.NextCursor != "cursor-2" {
		t.Fatalf("summary = %#v", summary)
	}
}
