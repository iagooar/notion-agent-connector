package notion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListBlockChildrenPageRawPassesPagination(t *testing.T) {
	var path string

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/blocks/block-1/children", func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[{"id":"child-1","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Hi"}]}}],"has_more":true,"next_cursor":"cursor-2"}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	result, err := client.ListBlockChildrenPageRaw(context.Background(), "block-1", "cursor-1", 25)
	if err != nil {
		t.Fatalf("ListBlockChildrenPageRaw() error = %v", err)
	}
	if path != "page_size=25&start_cursor=cursor-1" && path != "start_cursor=cursor-1&page_size=25" {
		t.Fatalf("raw query = %q", path)
	}
	if !result.HasMore || result.NextCursor != "cursor-2" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAppendBlockChildrenRawUsesPositionPayload(t *testing.T) {
	var payload map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/blocks/block-1/children", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(`{"results":[{"id":"child-2","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Inserted"}]}}],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err := client.AppendBlockChildrenRaw(context.Background(), "block-1", []map[string]any{
		{"object": "block", "type": "paragraph", "paragraph": map[string]any{"rich_text": []any{map[string]any{"type": "text", "text": map[string]any{"content": "Inserted"}}}}},
	}, AppendPosition{Kind: "after_block", AfterBlockID: "child-1"})
	if err != nil {
		t.Fatalf("AppendBlockChildrenRaw() error = %v", err)
	}
	position, _ := payload["position"].(map[string]any)
	if position["type"] != "after_block" {
		t.Fatalf("position = %#v", position)
	}
}

func TestSearchWorkspacePassesPayload(t *testing.T) {
	var payload map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(`{"results":[{"object":"page","id":"page-1","url":"https://www.notion.so/page1","properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Connector"}]}}}],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	result, err := client.SearchWorkspace(context.Background(), SearchOptions{
		Query:       "Connector",
		Filter:      map[string]any{"property": "object", "value": "page"},
		Sort:        map[string]any{"direction": "ascending", "timestamp": "last_edited_time"},
		StartCursor: "cursor-1",
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("SearchWorkspace() error = %v", err)
	}
	if payload["query"] != "Connector" || payload["page_size"] != float64(10) || payload["start_cursor"] != "cursor-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(result.Results) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestListCommentsUsesBlockIDAndPagination(t *testing.T) {
	var rawQuery string

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/comments", func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[{"id":"comment-1","discussion_id":"disc-1","rich_text":[{"plain_text":"Looks good"}],"created_time":"2026-03-17T10:00:00.000Z","created_by":{"id":"user-1","name":"Mati"},"parent":{"type":"page_id","page_id":"page-1"}}],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	result, err := client.ListComments(context.Background(), "page-1", "cursor-1", 5)
	if err != nil {
		t.Fatalf("ListComments() error = %v", err)
	}
	if rawQuery != "block_id=page-1&page_size=5&start_cursor=cursor-1" && rawQuery != "block_id=page-1&start_cursor=cursor-1&page_size=5" && rawQuery != "page_size=5&block_id=page-1&start_cursor=cursor-1" {
		t.Fatalf("raw query = %q", rawQuery)
	}
	summary := SummarizeCommentList(result)
	if len(summary.Comments) != 1 || summary.Comments[0].Text != "Looks good" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestCreateCommentUsesDiscussionID(t *testing.T) {
	var payload map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/comments", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(`{"id":"comment-2","discussion_id":"disc-1","rich_text":[{"plain_text":"Reply"}],"parent":{"type":"page_id","page_id":"page-1"}}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err := client.CreateComment(context.Background(), CommentCreateInput{
		DiscussionID: "disc-1",
		RichText:     RichTextInputFromText("Reply"),
	})
	if err != nil {
		t.Fatalf("CreateComment() error = %v", err)
	}
	if payload["discussion_id"] != "disc-1" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestSearchRootFindsPagesAndDatabases(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/root-page", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"root-page","url":"https://www.notion.so/root","properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Root Manual"}]}}}`))
	})
	mux.HandleFunc("/v1/pages/child-page", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"child-page","url":"https://www.notion.so/child","properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Connector Notes"}]}}}`))
	})
	mux.HandleFunc("/v1/blocks/root-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"child-page","type":"child_page","child_page":{"title":"Connector Notes"}},{"id":"db-1","type":"child_database","child_database":{"title":"Verification DB"}}],"has_more":false,"next_cursor":null}`))
	})
	mux.HandleFunc("/v1/blocks/child-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	result, err := client.SearchRoot(context.Background(), "root-page", RootSearchOptions{Query: "connector", MaxDepth: 2})
	if err != nil {
		t.Fatalf("SearchRoot() error = %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].Title != "Connector Notes" {
		t.Fatalf("result = %#v", result)
	}
}
