package notion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdateDocumentUsesMarkdownReplaceByDefault(t *testing.T) {
	var (
		sawMarkdownPatch bool
		sawBlocksPatch   bool
		payload          map[string]any
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Live page"}]}}}`))
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"live-page"}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nOld content","truncated":false,"unknown_block_ids":[]}`))
		case http.MethodPatch:
			sawMarkdownPatch = true
			_ = json.NewDecoder(r.Body).Decode(&payload)
			_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nFresh content","truncated":false,"unknown_block_ids":[]}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		sawBlocksPatch = true
		t.Fatalf("unexpected block mutation via %s", r.Method)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	doc, err := ParseMarkdown("# Manual\n\nFresh content")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err = client.UpdateDocument(context.Background(), "live-page", doc, false, false)
	if err != nil {
		t.Fatalf("UpdateDocument() error = %v", err)
	}

	if !sawMarkdownPatch {
		t.Fatal("expected markdown PATCH request")
	}
	if sawBlocksPatch {
		t.Fatal("did not expect block append request")
	}
	if payload["type"] != "replace_content" {
		t.Fatalf("type = %#v", payload["type"])
	}
	body, _ := payload["replace_content"].(map[string]any)
	if got := body["new_str"]; got != "# Live page\n\nFresh content" {
		t.Fatalf("new_str = %#v", got)
	}
}

func TestUpdateDocumentSectionUsesUpdateContent(t *testing.T) {
	var payload map[string]any

	currentMarkdown := "# Live page\n\nIntro\n\n## Operator setup\n\nOld steps.\n\n## Local configuration\n\nOld config."
	updatedMarkdown := "# Live page\n\nIntro\n\n## Operator setup\n\nNew steps.\n\n## Local configuration\n\nOld config."

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Live page"}]}}}`))
	})
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"live-page","markdown":"` + escapeJSON(currentMarkdown) + `","truncated":false,"unknown_block_ids":[]}`))
		case http.MethodPatch:
			_ = json.NewDecoder(r.Body).Decode(&payload)
			_, _ = w.Write([]byte(`{"id":"live-page","markdown":"` + escapeJSON(updatedMarkdown) + `","truncated":false,"unknown_block_ids":[]}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	doc, err := ParseMarkdown("# Manual\n\nIntro\n\n## Operator setup\n\nNew steps.\n\n## Local configuration\n\nOld config.")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err = client.UpdateDocumentSection(context.Background(), "live-page", doc, "Operator setup", 2, false)
	if err != nil {
		t.Fatalf("UpdateDocumentSection() error = %v", err)
	}

	if payload["type"] != "update_content" {
		t.Fatalf("type = %#v", payload["type"])
	}
	body, _ := payload["update_content"].(map[string]any)
	updates, _ := body["content_updates"].([]any)
	if len(updates) != 1 {
		t.Fatalf("len(content_updates) = %d", len(updates))
	}
	update, _ := updates[0].(map[string]any)
	if got := update["old_str"]; got != "## Operator setup\n\nOld steps." {
		t.Fatalf("old_str = %#v", got)
	}
	if got := update["new_str"]; got != "## Operator setup\n\nNew steps." {
		t.Fatalf("new_str = %#v", got)
	}
}

func TestUpdateDocumentSectionWithLocalMediaReplacesOnlyTargetSection(t *testing.T) {
	var (
		appendPayloads []map[string]any
		deletedBlocks  []string
		listCount      int
	)

	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "setup.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	initialBlocks := `{"results":[
		{"id":"intro","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Intro text"}]}},
		{"id":"setup-heading","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Operator setup"}]}},
		{"id":"setup-old","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Old steps."}]}},
		{"id":"config-heading","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Local configuration"}]}},
		{"id":"config-body","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Keep me."}]}}
	],"has_more":false,"next_cursor":null}`
	updatedBlocks := `{"results":[
		{"id":"intro","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Intro text"}]}},
		{"id":"setup-heading-new","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Operator setup"}]}},
		{"id":"setup-new","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"New steps."}]}},
		{"id":"setup-image","type":"image","image":{"type":"file","caption":[{"plain_text":"Setup image"}],"file":{"url":"https://example.com/setup.png"}}},
		{"id":"config-heading","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Local configuration"}]}},
		{"id":"config-body","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Keep me."}]}}
	],"has_more":false,"next_cursor":null}`

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listCount++
			if listCount == 1 {
				_, _ = w.Write([]byte(initialBlocks))
				return
			}
			_, _ = w.Write([]byte(updatedBlocks))
		case http.MethodPatch:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			appendPayloads = append(appendPayloads, body)
			_, _ = w.Write([]byte(`{"results":[{"id":"setup-heading-new"},{"id":"setup-new"},{"id":"setup-image"}]}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/blocks/setup-heading", func(w http.ResponseWriter, r *http.Request) {
		deletedBlocks = append(deletedBlocks, "setup-heading")
		_, _ = w.Write([]byte(`{"id":"setup-heading","archived":true}`))
	})
	mux.HandleFunc("/v1/blocks/setup-old", func(w http.ResponseWriter, r *http.Request) {
		deletedBlocks = append(deletedBlocks, "setup-old")
		_, _ = w.Write([]byte(`{"id":"setup-old","archived":true}`))
	})
	mux.HandleFunc("/v1/file_uploads", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"upload-1"}`))
	})
	mux.HandleFunc("/v1/file_uploads/upload-1/send", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"uploaded"}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	doc, err := ParseMarkdown("# Manual\n\n## Operator setup\n\nNew steps.\n\n![Setup image](" + imagePath + ")\n\n## Local configuration\n\nKeep me.")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	result, err := client.UpdateDocumentSection(context.Background(), "live-page", doc, "Operator setup", 2, false)
	if err != nil {
		t.Fatalf("UpdateDocumentSection() error = %v", err)
	}

	if result.BlockCount != 3 {
		t.Fatalf("BlockCount = %d", result.BlockCount)
	}
	if len(appendPayloads) != 1 {
		t.Fatalf("len(appendPayloads) = %d", len(appendPayloads))
	}
	position, _ := appendPayloads[0]["position"].(map[string]any)
	if position["type"] != "after_block" {
		t.Fatalf("position = %#v", position)
	}
	afterBlock, _ := position["after_block"].(map[string]any)
	if afterBlock["id"] != "intro" {
		t.Fatalf("after_block.id = %#v", afterBlock["id"])
	}
	if len(deletedBlocks) != 2 {
		t.Fatalf("deletedBlocks = %#v", deletedBlocks)
	}
	if !containsString(deletedBlocks, "setup-heading") || !containsString(deletedBlocks, "setup-old") {
		t.Fatalf("deletedBlocks = %#v", deletedBlocks)
	}
}

func TestUpdateDocumentWithLocalImagesAppendsBeforeDeletingExistingBlocks(t *testing.T) {
	var calls []string

	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "setup.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Live page"}]}}}`))
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"live-page"}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			calls = append(calls, "list")
			_, _ = w.Write([]byte(`{"results":[{"id":"old-1","type":"paragraph"}],"has_more":false,"next_cursor":null}`))
		case http.MethodPatch:
			calls = append(calls, "append")
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/blocks/old-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method %s", r.Method)
		}
		calls = append(calls, "delete")
		_, _ = w.Write([]byte(`{"id":"old-1","archived":true}`))
	})
	mux.HandleFunc("/v1/file_uploads", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"upload-1"}`))
	})
	mux.HandleFunc("/v1/file_uploads/upload-1/send", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"uploaded"}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	doc, err := ParseMarkdown("# Manual\n\nUpdated text.\n\n![Setup image](" + imagePath + ")")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err = client.UpdateDocument(context.Background(), "live-page", doc, false, false)
	if err != nil {
		t.Fatalf("UpdateDocument() error = %v", err)
	}

	got := strings.Join(calls, ",")
	if got != "list,append,delete" {
		t.Fatalf("calls = %q", got)
	}
}

func TestUpdateDocumentSeedsEmptyPageWithBlocksForLocalImages(t *testing.T) {
	var (
		sawMarkdownPatch bool
		appended         []map[string]any
	)

	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "setup.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Live page"}]}}}`))
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"live-page"}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page","truncated":false,"unknown_block_ids":[]}`))
		case http.MethodPatch:
			sawMarkdownPatch = true
			t.Fatalf("unexpected markdown PATCH for empty local-image page")
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
		case http.MethodPatch:
			var body struct {
				Children []map[string]any `json:"children"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			appended = append(appended, body.Children...)
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/file_uploads", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"upload-1"}`))
	})
	mux.HandleFunc("/v1/file_uploads/upload-1/send", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"uploaded"}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	doc, err := ParseMarkdown("# Manual\n\nUse `support_ticket` here.\n\n![Setup image](" + imagePath + ")")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err = client.UpdateDocument(context.Background(), "live-page", doc, false, false)
	if err != nil {
		t.Fatalf("UpdateDocument() error = %v", err)
	}

	if sawMarkdownPatch {
		t.Fatal("did not expect markdown patch for empty page with local images")
	}
	if len(appended) != 2 {
		t.Fatalf("len(appended) = %d", len(appended))
	}
	paragraph, _ := appended[0]["paragraph"].(map[string]any)
	rich, _ := paragraph["rich_text"].([]any)
	if len(rich) != 3 {
		t.Fatalf("len(rich_text) = %d", len(rich))
	}
	second, _ := rich[1].(map[string]any)
	annotations, _ := second["annotations"].(map[string]any)
	if annotations["code"] != true {
		t.Fatalf("expected code annotation, got %#v", annotations)
	}
	image, _ := appended[1]["image"].(map[string]any)
	fileUpload, _ := image["file_upload"].(map[string]any)
	if fileUpload["id"] != "upload-1" {
		t.Fatalf("file_upload.id = %#v", fileUpload["id"])
	}
}

func TestUpdateDocumentSeedsEmptyPageWithNativeMediaBlocks(t *testing.T) {
	var (
		sawMarkdownPatch bool
		appended         []map[string]any
	)

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "spec.pdf")
	audioPath := filepath.Join(tmpDir, "intro.mp3")
	if err := os.WriteFile(pdfPath, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(audioPath, []byte("mp3"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	uploadIDs := []string{"upload-pdf", "upload-audio"}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Live page"}]}}}`))
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"live-page"}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page","truncated":false,"unknown_block_ids":[]}`))
		case http.MethodPatch:
			sawMarkdownPatch = true
			t.Fatalf("unexpected markdown PATCH for empty local-media page")
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
		case http.MethodPatch:
			var body struct {
				Children []map[string]any `json:"children"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			appended = append(appended, body.Children...)
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/file_uploads", func(w http.ResponseWriter, r *http.Request) {
		id := uploadIDs[0]
		uploadIDs = uploadIDs[1:]
		_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
	})
	mux.HandleFunc("/v1/file_uploads/upload-pdf/send", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"uploaded"}`))
	})
	mux.HandleFunc("/v1/file_uploads/upload-audio/send", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"uploaded"}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	doc, err := ParseMarkdown("# Manual\n\n<pdf src=\"" + pdfPath + "\">Spec</pdf>\n\n<audio src=\"" + audioPath + "\">Intro</audio>")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err = client.UpdateDocument(context.Background(), "live-page", doc, false, false)
	if err != nil {
		t.Fatalf("UpdateDocument() error = %v", err)
	}

	if sawMarkdownPatch {
		t.Fatal("did not expect markdown patch for empty page with local media")
	}
	if len(appended) != 2 {
		t.Fatalf("len(appended) = %d", len(appended))
	}
	pdf, _ := appended[0]["pdf"].(map[string]any)
	pdfUpload, _ := pdf["file_upload"].(map[string]any)
	if pdfUpload["id"] != "upload-pdf" {
		t.Fatalf("pdf file_upload.id = %#v", pdfUpload["id"])
	}
	audio, _ := appended[1]["audio"].(map[string]any)
	audioUpload, _ := audio["file_upload"].(map[string]any)
	if audioUpload["id"] != "upload-audio" {
		t.Fatalf("audio file_upload.id = %#v", audioUpload["id"])
	}
}

func TestSendFileUploadRequiresUploadedStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/file_uploads/upload-1/send", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"processing"}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	tmpDir := t.TempDir()
	filePath := tmpDir + "/image.png"
	if err := os.WriteFile(filePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	err := client.sendFileUpload(context.Background(), "upload-1", filePath, "image.png", "image/png")
	if err == nil {
		t.Fatal("expected error for non-uploaded file status")
	}
	if !strings.Contains(err.Error(), `notion file upload status = "processing"`) {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestUploadFileUsesMultipartForLargeFiles(t *testing.T) {
	var (
		createPayload map[string]any
		partNumbers   []string
		completed     bool
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/file_uploads", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&createPayload)
		_, _ = w.Write([]byte(`{"id":"upload-1"}`))
	})
	mux.HandleFunc("/v1/file_uploads/upload-1/send", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		partNumbers = append(partNumbers, r.FormValue("part_number"))
		_, _ = w.Write([]byte(`{"status":"pending"}`))
	})
	mux.HandleFunc("/v1/file_uploads/upload-1/complete", func(w http.ResponseWriter, r *http.Request) {
		completed = true
		_, _ = w.Write([]byte(`{"status":"uploaded"}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "manual.pdf")
	if err := os.WriteFile(filePath, make([]byte, int(maxSinglePartUploadSize+1)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	uploadID, err := client.uploadFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("uploadFile() error = %v", err)
	}
	if uploadID != "upload-1" {
		t.Fatalf("uploadID = %q", uploadID)
	}
	if createPayload["mode"] != "multi_part" {
		t.Fatalf("mode = %#v", createPayload["mode"])
	}
	if createPayload["number_of_parts"] != float64(3) {
		t.Fatalf("number_of_parts = %#v", createPayload["number_of_parts"])
	}
	if strings.Join(partNumbers, ",") != "1,2,3" {
		t.Fatalf("partNumbers = %q", strings.Join(partNumbers, ","))
	}
	if !completed {
		t.Fatal("expected multipart upload completion")
	}
}

func TestReadPageTreeUsesMarkdownByDefault(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("include_transcript"); got != "" {
			t.Fatalf("include_transcript = %q", got)
		}
		_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nMarkdown body","truncated":false,"unknown_block_ids":[]}`))
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	tree, err := client.ReadPageTree(context.Background(), "live-page", 0)
	if err != nil {
		t.Fatalf("ReadPageTree() error = %v", err)
	}
	if tree.Title != "Live page" {
		t.Fatalf("Title = %q", tree.Title)
	}
	if tree.Content != "# Live page\n\nMarkdown body" {
		t.Fatalf("Content = %q", tree.Content)
	}
}

func TestReadPageTreeWithOptionsIncludesTranscriptQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("include_transcript"); got != "true" {
			t.Fatalf("include_transcript = %q", got)
		}
		_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nTranscript body","truncated":false,"unknown_block_ids":[]}`))
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	tree, err := client.ReadPageTreeWithOptions(context.Background(), "live-page", 0, ReadOptions{
		Mode:              ReadModeMarkdown,
		IncludeTranscript: true,
	})
	if err != nil {
		t.Fatalf("ReadPageTreeWithOptions() error = %v", err)
	}
	if tree.Content != "# Live page\n\nTranscript body" {
		t.Fatalf("Content = %q", tree.Content)
	}
}

func TestReadPageTreeRecoversUnknownAliasBlockIntoMarkdown(t *testing.T) {
	const refPageID = "324c89e1-ec89-80f3-8b44-e4b064fff562"
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nBefore\n<unknown url=\"https://www.notion.so/live-page#alias-1\" alt=\"alias\"/>\nAfter","truncated":true,"unknown_block_ids":["alias-1"]}`))
	})
	mux.HandleFunc("/v1/pages/alias-1/markdown", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"object":"error","code":"object_not_found"}`, http.StatusNotFound)
	})
	mux.HandleFunc("/v1/blocks/alias-1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"alias-1","type":"link_to_page","has_children":false,"link_to_page":{"type":"page_id","page_id":"` + refPageID + `"}}`))
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	tree, err := client.ReadPageTree(context.Background(), "live-page", 0)
	if err != nil {
		t.Fatalf("ReadPageTree() error = %v", err)
	}
	if !strings.Contains(tree.Content, `<page url="https://www.notion.so/324c89e1ec8980f38b44e4b064fff562">324c89e1-ec89-80f3-8b44-e4b064fff562</page>`) {
		t.Fatalf("Content = %q", tree.Content)
	}
}

func TestReadPageTreeRecoversSelfReferentialAliasBlockIntoMarkdown(t *testing.T) {
	const refPageID = "324c89e1-ec89-80f3-8b44-e4b064fff562"
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nBefore\n<unknown url=\"https://www.notion.so/live-page#alias-1\" alt=\"alias\"/>\nAfter","truncated":true,"unknown_block_ids":["alias-1"]}`))
	})
	mux.HandleFunc("/v1/pages/alias-1/markdown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"alias-1","markdown":"<unknown url=\"https://www.notion.so/alias-1#alias-1\" alt=\"alias\"/>","truncated":true,"unknown_block_ids":["alias-1"]}`))
	})
	mux.HandleFunc("/v1/blocks/alias-1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"alias-1","type":"link_to_page","has_children":false,"link_to_page":{"type":"page_id","page_id":"` + refPageID + `"}}`))
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	tree, err := client.ReadPageTree(context.Background(), "live-page", 0)
	if err != nil {
		t.Fatalf("ReadPageTree() error = %v", err)
	}
	if !strings.Contains(tree.Content, `<page url="https://www.notion.so/324c89e1ec8980f38b44e4b064fff562">324c89e1-ec89-80f3-8b44-e4b064fff562</page>`) {
		t.Fatalf("Content = %q", tree.Content)
	}
}

func TestRenderBlocksMarkdownUsesNativeEnhancedForms(t *testing.T) {
	rendered := strings.Join(renderBlocksMarkdown([]notionBlock{
		{
			Type: "callout",
			Callout: &struct {
				RichText []richTextItem `json:"rich_text"`
				Color    string         `json:"color,omitempty"`
				Icon     *struct {
					Type  string `json:"type"`
					Emoji string `json:"emoji,omitempty"`
				} `json:"icon,omitempty"`
			}{
				RichText: []richTextItem{{
					PlainText: "Check ",
				}, {
					PlainText: "Notion Go Connector",
					Mention: &struct {
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
					}{
						Type: "page",
						Page: &struct {
							ID string `json:"id"`
						}{ID: "324c89e1-ec89-80f3-8b44-e4b064fff562"},
					},
				}},
				Color: "yellow_background",
				Icon: &struct {
					Type  string `json:"type"`
					Emoji string `json:"emoji,omitempty"`
				}{Type: "emoji", Emoji: "💡"},
			},
		},
		{
			Type: "table",
			Table: &struct {
				TableWidth      int  `json:"table_width"`
				HasColumnHeader bool `json:"has_column_header"`
				HasRowHeader    bool `json:"has_row_header"`
			}{TableWidth: 2, HasColumnHeader: true, HasRowHeader: false},
			Children: []notionBlock{
				{Type: "table_row", TableRow: &struct {
					Cells [][]richTextItem `json:"cells"`
				}{Cells: [][]richTextItem{{{PlainText: "Primitive"}}, {{PlainText: "Status"}}}}},
				{Type: "table_row", TableRow: &struct {
					Cells [][]richTextItem `json:"cells"`
				}{Cells: [][]richTextItem{{{PlainText: "Tables"}}, {{PlainText: "Working"}}}}},
			},
		},
	}, 0, ""), "\n")

	if !strings.Contains(rendered, `<callout icon="💡" color="yellow_background">`) {
		t.Fatalf("rendered = %q", rendered)
	}
	if !strings.Contains(rendered, `<mention-page url="https://www.notion.so/324c89e1ec8980f38b44e4b064fff562">Notion Go Connector</mention-page>`) {
		t.Fatalf("rendered = %q", rendered)
	}
	if !strings.Contains(rendered, `<table header-row="true" header-column="false">`) {
		t.Fatalf("rendered = %q", rendered)
	}
}

func TestRenderBlocksMarkdownRendersExtendedNativeBlocks(t *testing.T) {
	rendered := strings.Join(renderBlocksMarkdown([]notionBlock{
		{
			Type: "table_of_contents",
			TableOfContents: &struct {
				Color string `json:"color,omitempty"`
			}{Color: "gray"},
		},
		{
			Type: "bookmark",
			Bookmark: &struct {
				URL string `json:"url"`
			}{URL: "https://example.com/spec"},
		},
		{
			Type: "column_list",
			Children: []notionBlock{{
				Type: "column",
				Children: []notionBlock{{
					Type: "paragraph",
					Paragraph: &struct {
						RichText []richTextItem `json:"rich_text"`
					}{RichText: []richTextItem{{PlainText: "Left"}}},
				}},
			}},
		},
		{
			Type: "synced_block",
			SyncedBlock: &struct {
				SyncedFrom *struct {
					Type    string `json:"type"`
					BlockID string `json:"block_id,omitempty"`
				} `json:"synced_from,omitempty"`
			}{SyncedFrom: &struct {
				Type    string `json:"type"`
				BlockID string `json:"block_id,omitempty"`
			}{Type: "block_id", BlockID: "sync-1"}},
		},
	}, 0, ""), "\n")
	if !strings.Contains(rendered, `<table-of-contents color="gray"/>`) {
		t.Fatalf("rendered = %q", rendered)
	}
	if !strings.Contains(rendered, `<bookmark url="https://example.com/spec"/>`) {
		t.Fatalf("rendered = %q", rendered)
	}
	if !strings.Contains(rendered, "<columns>") || !strings.Contains(rendered, "<column>") {
		t.Fatalf("rendered = %q", rendered)
	}
	if !strings.Contains(rendered, `<synced-block block-id="sync-1"/>`) {
		t.Fatalf("rendered = %q", rendered)
	}
}

func TestReadPageTreeFallsBackToBlocksWhenMarkdownUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"object":"error","status":404,"code":"object_not_found"}`, http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err := client.ReadPageTree(context.Background(), "live-page", 0)
	if err == nil {
		t.Fatal("expected markdown read error")
	}
	if !strings.Contains(err.Error(), "object_not_found") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestReadPageTreeBlocksModeAvoidsDuplicateTitleAndPreservesInlineCode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Live page"}]}}}`))
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"h1","type":"heading_1","heading_1":{"rich_text":[{"plain_text":"Live page"}]}},{"id":"p1","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Use ","annotations":{"code":false}},{"plain_text":"direnv allow","annotations":{"code":true}}]}}],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	tree, err := client.ReadPageTreeWithMode(context.Background(), "live-page", 0, ReadModeBlocks)
	if err != nil {
		t.Fatalf("ReadPageTreeWithMode() error = %v", err)
	}
	if strings.Count(tree.Content, "# Live page") != 1 {
		t.Fatalf("Content duplicated title: %q", tree.Content)
	}
	if !strings.Contains(tree.Content, "`direnv allow`") {
		t.Fatalf("Content missing inline code annotation: %q", tree.Content)
	}
}

func TestReadPageTreeBlocksModeRendersNativeMediaBlocks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Live page"}]}}}`))
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"id":"img-1","type":"image","image":{"type":"external","caption":[{"plain_text":"Setup"}],"external":{"url":"https://example.com/setup.png"}}},
			{"id":"pdf-1","type":"pdf","pdf":{"type":"file","caption":[{"plain_text":"Spec"}],"file":{"url":"https://example.com/spec.pdf"}}},
			{"id":"file-1","type":"file","file":{"type":"external","caption":[{"plain_text":"Archive"}],"external":{"url":"https://example.com/archive.zip"}}},
			{"id":"audio-1","type":"audio","audio":{"type":"file","caption":[{"plain_text":"Intro"}],"file":{"url":"https://example.com/intro.mp3"}}},
			{"id":"video-1","type":"video","video":{"type":"external","caption":[{"plain_text":"Demo"}],"external":{"url":"https://example.com/demo.mp4"}}}
		],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	tree, err := client.ReadPageTreeWithMode(context.Background(), "live-page", 0, ReadModeBlocks)
	if err != nil {
		t.Fatalf("ReadPageTreeWithMode() error = %v", err)
	}
	for _, want := range []string{
		"![Setup](https://example.com/setup.png)",
		`<pdf src="https://example.com/spec.pdf">Spec</pdf>`,
		`<file src="https://example.com/archive.zip">Archive</file>`,
		`<audio src="https://example.com/intro.mp3">Intro</audio>`,
		`<video src="https://example.com/demo.mp4">Demo</video>`,
	} {
		if !strings.Contains(tree.Content, want) {
			t.Fatalf("Content missing %q: %s", want, tree.Content)
		}
	}
}

func TestReadPageTreeBlocksModeRendersRichBlocksAndInlineFormatting(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Live page"}]}}}`))
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"id":"todo-1","type":"to_do","to_do":{"checked":true,"rich_text":[{"plain_text":"Done task"}]}},
			{"id":"callout-1","type":"callout","callout":{"icon":{"type":"emoji","emoji":"⚠️"},"rich_text":[{"plain_text":"Watch this"}]}},
			{"id":"divider-1","type":"divider","divider":{}},
			{"id":"equation-1","type":"equation","equation":{"expression":"a^2+b^2=c^2"}},
			{"id":"p1","type":"paragraph","paragraph":{"rich_text":[
				{"plain_text":"Bold","href":"","annotations":{"bold":true,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"}},
				{"plain_text":" and ","href":"","annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"}},
				{"plain_text":"docs","href":"https://example.com","annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"}}
			]}}
		],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	tree, err := client.ReadPageTreeWithMode(context.Background(), "live-page", 0, ReadModeBlocks)
	if err != nil {
		t.Fatalf("ReadPageTreeWithMode() error = %v", err)
	}
	for _, want := range []string{
		"- [x] Done task",
		"<callout icon=\"⚠️\">",
		"\tWatch this",
		"</callout>",
		"---",
		"$$\na^2+b^2=c^2\n$$",
		"**Bold** and [docs](https://example.com)",
	} {
		if !strings.Contains(tree.Content, want) {
			t.Fatalf("Content missing %q: %s", want, tree.Content)
		}
	}
}

func TestReadPageTreeReturnsErrorWhenMarkdownTruncated(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nBody","truncated":true,"unknown_block_ids":[]}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err := client.ReadPageTree(context.Background(), "live-page", 0)
	if err == nil {
		t.Fatal("expected incomplete markdown error")
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestReadPageTreeResolvesUnknownMarkdownBlocks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nIntro\n\n<unknown url=\"https://notion.so/x\" alt=\"bookmark\"/>","truncated":true,"unknown_block_ids":["block-1"]}`))
	})
	mux.HandleFunc("/v1/pages/block-1/markdown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"block-1","markdown":"Recovered bookmark summary","truncated":false,"unknown_block_ids":[]}`))
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	tree, err := client.ReadPageTree(context.Background(), "live-page", 0)
	if err != nil {
		t.Fatalf("ReadPageTree() error = %v", err)
	}
	if tree.Content != "# Live page\n\nIntro\n\nRecovered bookmark summary" {
		t.Fatalf("Content = %q", tree.Content)
	}
}

func TestReadPageTreeGracefullyKeepsRecoverableUnknownBlocks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nIntro\n\n<unknown url=\"https://notion.so/x\" alt=\"bookmark\"/>","truncated":true,"unknown_block_ids":["block-1"]}`))
	})
	mux.HandleFunc("/v1/pages/block-1/markdown", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"object":"error","status":404,"code":"object_not_found"}`, http.StatusNotFound)
	})
	mux.HandleFunc("/v1/blocks/live-page/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	tree, err := client.ReadPageTree(context.Background(), "live-page", 0)
	if err != nil {
		t.Fatalf("ReadPageTree() error = %v", err)
	}
	if !strings.Contains(tree.Content, `<unknown-block id="block-1"/>`) {
		t.Fatalf("Content = %q", tree.Content)
	}
}

func TestUpsertDocumentCreatesChildPageWithMarkdown(t *testing.T) {
	var (
		createPayload     map[string]any
		sawMarkdownPatch  bool
		sawBlocksMutation bool
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/blocks/parent-page/children", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"results":[],"has_more":false,"next_cursor":null}`))
		case http.MethodPatch:
			sawBlocksMutation = true
			t.Fatalf("unexpected block patch for markdown child creation")
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/pages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&createPayload)
		_, _ = w.Write([]byte(`{"id":"child-page"}`))
	})
	mux.HandleFunc("/v1/pages/child-page", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Fresh page"}]}}}`))
	})
	mux.HandleFunc("/v1/pages/child-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"child-page","markdown":"# Fresh page\n\nBody","truncated":false,"unknown_block_ids":[]}`))
		case http.MethodPatch:
			sawMarkdownPatch = true
			t.Fatalf("unexpected markdown patch for markdown child creation")
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	doc, err := ParseMarkdown("# Fresh page\n\nBody")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	result, err := client.UpsertDocument(context.Background(), "parent-page", doc)
	if err != nil {
		t.Fatalf("UpsertDocument() error = %v", err)
	}

	if sawMarkdownPatch {
		t.Fatal("did not expect markdown patch")
	}
	if sawBlocksMutation {
		t.Fatal("did not expect block mutation")
	}
	if result.PageID != "child-page" || !result.Created {
		t.Fatalf("unexpected result = %#v", result)
	}
	if got := createPayload["markdown"]; got != "# Fresh page\n\nBody" {
		t.Fatalf("markdown = %#v", got)
	}
}

func TestCreateChildDocumentVerificationAllowsMissingReturnedTitleHeading(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"id":"child-page"}`))
	})
	mux.HandleFunc("/v1/pages/child-page", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Fresh page"}]}}}`))
	})
	mux.HandleFunc("/v1/pages/child-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"id":"child-page","markdown":"Body","truncated":false,"unknown_block_ids":[]}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	doc, err := ParseMarkdown("# Fresh page\n\nBody")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	result, err := client.CreateChildDocument(context.Background(), "parent-page", doc)
	if err != nil {
		t.Fatalf("CreateChildDocument() error = %v", err)
	}
	if !result.Created || result.PageID != "child-page" {
		t.Fatalf("unexpected result = %#v", result)
	}
}

func TestArchiveBlocksRunsDeletesConcurrently(t *testing.T) {
	var (
		active     int32
		maxActive  int32
		seenDelete sync.Map
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/blocks/old-1", func(w http.ResponseWriter, r *http.Request) {
		recordConcurrentDelete(t, r, &active, &maxActive, &seenDelete, "old-1")
		_, _ = w.Write([]byte(`{"id":"old-1","archived":true}`))
	})
	mux.HandleFunc("/v1/blocks/old-2", func(w http.ResponseWriter, r *http.Request) {
		recordConcurrentDelete(t, r, &active, &maxActive, &seenDelete, "old-2")
		_, _ = w.Write([]byte(`{"id":"old-2","archived":true}`))
	})
	mux.HandleFunc("/v1/blocks/old-3", func(w http.ResponseWriter, r *http.Request) {
		recordConcurrentDelete(t, r, &active, &maxActive, &seenDelete, "old-3")
		_, _ = w.Write([]byte(`{"id":"old-3","archived":true}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("token", server.Client(), server.URL+"/v1")
	err := client.archiveBlocks(context.Background(), []notionBlock{
		{ID: "old-1"},
		{ID: "old-2"},
		{ID: "old-3"},
	})
	if err != nil {
		t.Fatalf("archiveBlocks() error = %v", err)
	}
	if atomic.LoadInt32(&maxActive) < 2 {
		t.Fatalf("maxActive = %d, want concurrent deletes", atomic.LoadInt32(&maxActive))
	}
	for _, id := range []string{"old-1", "old-2", "old-3"} {
		if _, ok := seenDelete.Load(id); !ok {
			t.Fatalf("missing delete for %s", id)
		}
	}
}

func TestUpdateDocumentVerificationErrorIncludesDiffSummary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pages/live-page", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"properties":{"title":{"type":"title","title":[{"plain_text":"Live page"}]}}}`))
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"live-page"}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/v1/pages/live-page/markdown", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"live-page","markdown":"# Live page\n\nDifferent content","truncated":false,"unknown_block_ids":[]}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	doc, err := ParseMarkdown("# Manual\n\nFresh content")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	client := NewClient("token", server.Client(), server.URL+"/v1")
	_, err = client.UpdateDocument(context.Background(), "live-page", doc, false, false)
	if err == nil {
		t.Fatal("expected verification error")
	}
	if !strings.Contains(err.Error(), `first mismatch at line 2`) {
		t.Fatalf("unexpected error = %v", err)
	}
	if !strings.Contains(err.Error(), `"Fresh content"`) {
		t.Fatalf("unexpected error = %v", err)
	}
}

func escapeJSON(input string) string {
	encoded, _ := json.Marshal(input)
	return strings.Trim(string(encoded), `"`)
}

func recordConcurrentDelete(t *testing.T, r *http.Request, active *int32, maxActive *int32, seenDelete *sync.Map, id string) {
	t.Helper()
	if r.Method != http.MethodDelete {
		t.Fatalf("unexpected method %s", r.Method)
	}
	seenDelete.Store(id, true)
	current := atomic.AddInt32(active, 1)
	for {
		previous := atomic.LoadInt32(maxActive)
		if current <= previous || atomic.CompareAndSwapInt32(maxActive, previous, current) {
			break
		}
	}
	time.Sleep(40 * time.Millisecond)
	atomic.AddInt32(active, -1)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
