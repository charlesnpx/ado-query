package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type staticTokenProvider string

func (p staticTokenProvider) Token(context.Context) (string, error) {
	return string(p), nil
}

func TestDiscoverAttachmentsFromHTMLAndRelations(t *testing.T) {
	workItem := map[string]any{
		"fields":    map[string]any{"System.Description": `<img src="https://dev.azure.com/o/p/_apis/wit/attachments/abc?fileName=one.png">`},
		"relations": []any{map[string]any{"rel": "AttachedFile", "url": "https://dev.azure.com/o/p/_apis/wit/attachments/def?fileName=two.docx", "attributes": map[string]any{"name": "two.docx"}}},
	}
	comments := map[string]any{"comments": []any{map[string]any{"id": 7, "text": `<a href="https://dev.azure.com/o/p/_apis/wit/attachments/abc?fileName=one.png">one</a>`}}}
	attachments := discoverAttachments(workItem, comments)
	if len(attachments) != 2 || attachments[0].GUID != "abc" || attachments[1].GUID != "def" {
		t.Fatalf("attachments = %+v", attachments)
	}
	if got := attachments[0].Sources; len(got) != 2 || got[0] != "description" || got[1] != "comment:7" {
		t.Fatalf("sources = %+v", got)
	}
}

func TestFetchWorkItemWritesContentAndUsesCache(t *testing.T) {
	tmp := t.TempDir()
	markitdown := filepath.Join(tmp, "markitdown")
	if err := os.WriteFile(markitdown, []byte("#!/bin/sh\nsed 's/<[^>]*>//g' \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp+":/bin:/usr/bin")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/org/_apis/wit/workitems/123":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 123,
				"fields": map[string]any{
					"System.Title": "Example", "System.State": "Active", "System.Description": "<p>Hello</p>",
					"Microsoft.VSTS.Common.AcceptanceCriteria": "<p>Done</p>",
				},
				"relations": []any{map[string]any{"rel": "System.LinkTypes.Hierarchy-Forward", "url": serverURL(r) + "/org/_apis/wit/workItems/456"}},
			})
		case "/org/proj/_apis/wit/workItems/123/comments":
			_ = json.NewEncoder(w).Encode(map[string]any{"comments": []any{map[string]any{"id": 1, "text": "<p>Comment</p>"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	opts := queryOptions{id: "123", org: server.URL + "/org", project: "proj", outDir: filepath.Join(tmp, "out"), cacheDir: filepath.Join(tmp, "cache"), tokenProvider: staticTokenProvider("token")}
	content, err := fetchWorkItem(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fetchWorkItem(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if content.Fields.Description != "Hello" || len(content.Comments) != 1 || content.Relations.ChildIDs[0] != "456" {
		t.Fatalf("content = %+v", content)
	}
	for _, rel := range []string{"content.md", "content.json", "raw/work-item.json", "raw/comments.json"} {
		if _, err := os.Stat(filepath.Join(opts.outDir, rel)); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
}

func TestFetchWorkItemDefaultsOutputUnderCache(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "cwd")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/org/_apis/wit/workitems/123":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     123,
				"fields": map[string]any{"System.Title": "Example", "System.State": "Active"},
				"relations": []any{map[string]any{
					"rel":        "AttachedFile",
					"url":        serverURL(r) + "/org/_apis/wit/attachments/abc?fileName=one.bin",
					"attributes": map[string]any{"name": "one.bin"},
				}},
			})
		case "/org/proj/_apis/wit/workItems/123/comments":
			_ = json.NewEncoder(w).Encode(map[string]any{"comments": []any{}})
		case "/org/_apis/wit/attachments/abc":
			_, _ = w.Write([]byte("attachment"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts := queryOptions{
		id: "123", org: server.URL + "/org", project: "proj",
		cacheDir: filepath.Join(tmp, "cache"), includeAttachments: true, tokenProvider: staticTokenProvider("token"),
	}
	if _, err := fetchWorkItem(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	outDir := defaultOutputDir(opts)
	for _, path := range []string{
		filepath.Join(outDir, "content.md"),
		filepath.Join(outDir, "attachments", "abc__one.bin"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected cache-backed output %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(cwd, ".ado-query")); !os.IsNotExist(err) {
		t.Fatalf("default output created cwd .ado-query: %v", err)
	}
}

func TestFetchWorkItemTreeDefaultsOutputUnderCache(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "cwd")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/org/_apis/wit/workitems/123":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     123,
				"fields": map[string]any{"System.Title": "Root", "System.State": "Active"},
			})
		case "/org/proj/_apis/wit/workItems/123/comments":
			_ = json.NewEncoder(w).Encode(map[string]any{"comments": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts := queryOptions{
		id: "123", org: server.URL + "/org", project: "proj",
		cacheDir: filepath.Join(tmp, "cache"), tokenProvider: staticTokenProvider("token"),
	}
	if _, err := fetchWorkItemTree(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	opts.tree = true
	expectedRoot := filepath.Join(defaultOutputDir(opts), "content.md")
	expectedItem := filepath.Join(defaultOutputDir(opts), "items", "123", "content.md")
	for _, path := range []string{expectedRoot, expectedItem} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected cache-backed output %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(cwd, ".ado-query")); !os.IsNotExist(err) {
		t.Fatalf("default output created cwd .ado-query: %v", err)
	}
}

func TestMissingMarkitdownFallsBackToHTML(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	warnings := []string{}
	got := htmlToMarkdown("description", "<p>Hello</p>", &warnings)
	if !strings.Contains(got, "```html") || len(warnings) == 0 {
		t.Fatalf("got=%q warnings=%v", got, warnings)
	}
}

func TestSplitArgsAcceptsFlagsAfterID(t *testing.T) {
	flags, id, err := splitArgs([]string{"123", "--out", "/tmp/out", "--include-attachments"}, map[string]bool{"out": true})
	if err != nil {
		t.Fatal(err)
	}
	if id != "123" || len(flags) != 3 {
		t.Fatalf("id=%q flags=%v", id, flags)
	}
}

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
