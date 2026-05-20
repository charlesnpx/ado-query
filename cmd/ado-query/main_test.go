package main

import (
	"context"
	"encoding/json"
	"io"
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

func TestFetchRawQueryCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/org/proj/_apis/wit/workItems/123/comments":
			_ = json.NewEncoder(w).Encode(map[string]any{"comments": []any{map[string]any{"id": 1}}})
		case "/org/proj/_apis/git/repositories/repo/pullrequests":
			if r.URL.Query().Get("searchCriteria.status") != "completed" {
				t.Fatalf("status query = %q", r.URL.Query().Get("searchCriteria.status"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 1})
		case "/org/proj/_apis/git/repositories/repo/pullrequests/99":
			_ = json.NewEncoder(w).Encode(map[string]any{"pullRequestId": 99})
		case "/org/proj/_apis/git/repositories/repo/pullrequests/99/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts := queryOptions{org: server.URL + "/org", project: "proj", tokenProvider: staticTokenProvider("token")}
	opts.id = "123"
	for name, fn := range map[string]func() (json.RawMessage, error){
		"comments": func() (json.RawMessage, error) { return fetchComments(context.Background(), opts) },
		"pr-list": func() (json.RawMessage, error) {
			return fetchPullRequests(context.Background(), opts, "repo", "completed")
		},
		"pr-get": func() (json.RawMessage, error) { return fetchPullRequest(context.Background(), opts, "repo", "99") },
		"pr-threads": func() (json.RawMessage, error) {
			return fetchPullRequestThreads(context.Background(), opts, "repo", "99")
		},
	} {
		if body, err := fn(); err != nil || len(body) == 0 {
			t.Fatalf("%s body=%q err=%v", name, body, err)
		}
	}
}

func TestFetchWIQLSendsPostBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/org/proj/_apis/wit/wiql" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["query"] != "SELECT [System.Id] FROM WorkItems" {
			t.Fatalf("body = %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"workItems": []any{}})
	}))
	defer server.Close()

	opts := queryOptions{org: server.URL + "/org", project: "proj", tokenProvider: staticTokenProvider("token")}
	body, err := fetchWIQL(context.Background(), opts, "SELECT [System.Id] FROM WorkItems")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "workItems") {
		t.Fatalf("body = %s", body)
	}

	queryFile := filepath.Join(t.TempDir(), "query.wiql")
	if err := os.WriteFile(queryFile, []byte("SELECT * FROM WorkItems"), 0o644); err != nil {
		t.Fatal(err)
	}
	query, err := wiqlQueryText("@" + queryFile)
	if err != nil {
		t.Fatal(err)
	}
	if query != "SELECT * FROM WorkItems" {
		t.Fatalf("query = %q", query)
	}
}

func TestFetchAPIAcceptsFullURLWithoutOrg(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	opts := queryOptions{tokenProvider: staticTokenProvider("token")}
	body, err := fetchAPI(context.Background(), opts, server.URL+"/custom")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ok") {
		t.Fatalf("body = %s", body)
	}
}

func TestCodeSearchHelpers(t *testing.T) {
	if got := codeSearchURL("NPXInnovation", "7.1"); got != "https://almsearch.dev.azure.com/NPXInnovation/_apis/search/codesearchresults?api-version=7.1" {
		t.Fatalf("url = %q", got)
	}
	body := codeSearchBody("ECHO", "risk query", 25)
	if body["searchText"] != "risk query" || body["$top"] != 25 {
		t.Fatalf("body = %+v", body)
	}
	filters := body["filters"].(map[string][]string)
	if len(filters["Project"]) != 1 || filters["Project"][0] != "ECHO" {
		t.Fatalf("filters = %+v", filters)
	}
}

func TestDownloadURLWritesFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "attachment")
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "nested", "attachment.txt")
	opts := queryOptions{maxAttachmentBytes: 20, tokenProvider: staticTokenProvider("token")}
	if err := downloadURL(context.Background(), opts, server.URL, out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "attachment" {
		t.Fatalf("body = %q", body)
	}
}

func TestParseStandardQueryFlagsUsesEnvAndFlagsAfterPositionals(t *testing.T) {
	t.Setenv("ADO_ORG", "env-org")
	t.Setenv("ADO_PROJECT", "env-project")
	opts, positionals, err := parseStandardQueryFlags("pr-list", []string{"repo", "completed", "--project", "flag-project", "--api-version", "7.2"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if opts.org != "env-org" || opts.project != "flag-project" || opts.apiVersion != "7.2" {
		t.Fatalf("opts = %+v", opts)
	}
	if strings.Join(positionals, ",") != "repo,completed" {
		t.Fatalf("positionals = %+v", positionals)
	}
}

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
