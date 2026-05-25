package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const azureDevOpsResource = "499b84ac-1321-427f-aa17-267ca6975798"

type tokenProvider interface {
	Token(context.Context) (string, error)
}

type azureCLITokenProvider struct {
	resource string
}

func (p azureCLITokenProvider) Token(ctx context.Context) (string, error) {
	if _, err := exec.LookPath("az"); err != nil {
		return "", fmt.Errorf("Azure CLI executable az is required; install Azure CLI and run az login")
	}
	resource := p.resource
	if resource == "" {
		resource = azureDevOpsResource
	}
	cmd := exec.CommandContext(
		ctx,
		"az", "account", "get-access-token",
		"--resource", resource,
		"--query", "accessToken",
		"-o", "tsv",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = "run az login and verify this account can access Azure DevOps"
		}
		return "", fmt.Errorf("could not get Azure DevOps bearer token from Azure CLI: %s", detail)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("Azure CLI returned an empty Azure DevOps bearer token")
	}
	return token, nil
}

type adoClient struct {
	tokenProvider tokenProvider
	token         string
	http          *http.Client
}

type cacheMetadata struct {
	URL          string    `json:"url"`
	FetchedAt    time.Time `json:"fetchedAt"`
	ValidatedAt  time.Time `json:"validatedAt"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	StatusCode   int       `json:"statusCode,omitempty"`
	Size         int64     `json:"size"`
	Warning      string    `json:"warning,omitempty"`
}

type cachedJSONResult struct {
	Body    json.RawMessage
	Warning string
}

type cachedDownloadResult struct {
	Status  string
	Warning string
	Changed bool
}

func newADOClient(provider tokenProvider) *adoClient {
	if provider == nil {
		provider = azureCLITokenProvider{resource: azureDevOpsResource}
	}
	return &adoClient{tokenProvider: provider, http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *adoClient) fetchJSON(ctx context.Context, rawURL, cachePath string, noCache bool) (json.RawMessage, error) {
	result, err := c.fetchJSONCached(ctx, rawURL, cachePath, noCache)
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}

func (c *adoClient) fetchJSONCached(ctx context.Context, rawURL, cachePath string, noCache bool) (cachedJSONResult, error) {
	cachedBody, hasCache := readCacheBody(cachePath, noCache)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return cachedJSONResult{}, err
	}
	if err := c.authorize(ctx, req); err != nil {
		if hasCache {
			return c.staleJSONResult(rawURL, cachePath, cachedBody, 0, err), nil
		}
		return cachedJSONResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	if hasCache {
		readCacheMetadata(cachePath).applyValidators(req)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if hasCache {
			return c.staleJSONResult(rawURL, cachePath, cachedBody, 0, err), nil
		}
		return cachedJSONResult{}, err
	}
	defer res.Body.Close()
	now := time.Now().UTC()
	if res.StatusCode == http.StatusNotModified && hasCache {
		meta := readCacheMetadata(cachePath)
		meta.URL = firstNonEmpty(meta.URL, rawURL)
		if meta.FetchedAt.IsZero() {
			meta.FetchedAt = cacheFileModTime(cachePath)
		}
		meta.ValidatedAt = now
		meta.StatusCode = res.StatusCode
		meta.Size = int64(len(cachedBody))
		meta.Warning = ""
		updateValidatorsFromResponse(&meta, res)
		if err := writeCacheMetadata(cachePath, meta); err != nil {
			return cachedJSONResult{}, err
		}
		return cachedJSONResult{Body: json.RawMessage(cachedBody)}, nil
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return cachedJSONResult{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		err := fmt.Errorf("GET %s: %s: %s", rawURL, res.Status, strings.TrimSpace(string(body)))
		if hasCache {
			return c.staleJSONResult(rawURL, cachePath, cachedBody, res.StatusCode, err), nil
		}
		return cachedJSONResult{}, err
	}
	if !noCache && cachePath != "" {
		if err := writeRawJSON(cachePath, body); err != nil {
			return cachedJSONResult{}, err
		}
		if err := writeCacheMetadata(cachePath, metadataFromResponse(rawURL, res, int64(len(body)), now)); err != nil {
			return cachedJSONResult{}, err
		}
	}
	return cachedJSONResult{Body: json.RawMessage(body)}, nil
}

func (c *adoClient) postJSON(ctx context.Context, rawURL string, value any) (json.RawMessage, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err := c.authorize(ctx, req); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s: %s: %s", rawURL, res.Status, strings.TrimSpace(string(responseBody)))
	}
	return json.RawMessage(responseBody), nil
}

func (c *adoClient) download(ctx context.Context, rawURL, dst string, maxBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if err := c.authorize(ctx, req); err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("download %s: %s", rawURL, res.Status)
	}
	if res.ContentLength > maxBytes {
		return fmt.Errorf("attachment exceeds %d byte limit", maxBytes)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, &io.LimitedReader{R: res.Body, N: maxBytes + 1})
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if written > maxBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("attachment exceeds %d byte limit", maxBytes)
	}
	return os.Rename(tmp, dst)
}

func (c *adoClient) downloadCached(ctx context.Context, rawURL, cachePath string, maxBytes int64) (cachedDownloadResult, error) {
	hasCache := cacheFileExists(cachePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return cachedDownloadResult{}, err
	}
	if err := c.authorize(ctx, req); err != nil {
		if hasCache {
			return staleDownloadResult(rawURL, cachePath, 0, err), nil
		}
		return cachedDownloadResult{Status: "missing"}, err
	}
	if hasCache {
		readCacheMetadata(cachePath).applyValidators(req)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if hasCache {
			return staleDownloadResult(rawURL, cachePath, 0, err), nil
		}
		return cachedDownloadResult{Status: "missing"}, err
	}
	defer res.Body.Close()
	now := time.Now().UTC()
	if res.StatusCode == http.StatusNotModified && hasCache {
		meta := readCacheMetadata(cachePath)
		meta.URL = firstNonEmpty(meta.URL, rawURL)
		if meta.FetchedAt.IsZero() {
			meta.FetchedAt = cacheFileModTime(cachePath)
		}
		meta.ValidatedAt = now
		meta.StatusCode = res.StatusCode
		meta.Size = cacheFileSize(cachePath)
		meta.Warning = ""
		updateValidatorsFromResponse(&meta, res)
		if err := writeCacheMetadata(cachePath, meta); err != nil {
			return cachedDownloadResult{}, err
		}
		return cachedDownloadResult{Status: "validated"}, nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		err := fmt.Errorf("download %s: %s: %s", rawURL, res.Status, strings.TrimSpace(string(body)))
		if hasCache {
			return staleDownloadResult(rawURL, cachePath, res.StatusCode, err), nil
		}
		return cachedDownloadResult{Status: "missing"}, err
	}
	if res.ContentLength > maxBytes {
		err := fmt.Errorf("attachment exceeds %d byte limit", maxBytes)
		if hasCache {
			return staleDownloadResult(rawURL, cachePath, res.StatusCode, err), nil
		}
		return cachedDownloadResult{Status: "missing"}, err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return cachedDownloadResult{}, err
	}
	tmp := cachePath + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return cachedDownloadResult{}, err
	}
	written, copyErr := io.Copy(out, &io.LimitedReader{R: res.Body, N: maxBytes + 1})
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		if hasCache {
			return staleDownloadResult(rawURL, cachePath, res.StatusCode, copyErr), nil
		}
		return cachedDownloadResult{Status: "missing"}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		if hasCache {
			return staleDownloadResult(rawURL, cachePath, res.StatusCode, closeErr), nil
		}
		return cachedDownloadResult{Status: "missing"}, closeErr
	}
	if written > maxBytes {
		_ = os.Remove(tmp)
		err := fmt.Errorf("attachment exceeds %d byte limit", maxBytes)
		if hasCache {
			return staleDownloadResult(rawURL, cachePath, res.StatusCode, err), nil
		}
		return cachedDownloadResult{Status: "missing"}, err
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		if hasCache {
			return staleDownloadResult(rawURL, cachePath, res.StatusCode, err), nil
		}
		return cachedDownloadResult{Status: "missing"}, err
	}
	if err := writeCacheMetadata(cachePath, metadataFromResponse(rawURL, res, written, now)); err != nil {
		return cachedDownloadResult{}, err
	}
	return cachedDownloadResult{Status: "downloaded", Changed: true}, nil
}

func (c *adoClient) authorize(ctx context.Context, req *http.Request) error {
	if c.token == "" {
		token, err := c.tokenProvider.Token(ctx)
		if err != nil {
			return err
		}
		c.token = token
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return nil
}

func workItemURL(org, id, apiVersion string) string {
	return orgURL(org, fmt.Sprintf("/_apis/wit/workitems/%s?$expand=relations&api-version=%s", url.PathEscape(id), url.QueryEscape(apiVersion)))
}

func commentsURL(org, project, id string) string {
	return orgURL(org, fmt.Sprintf("/%s/_apis/wit/workItems/%s/comments?api-version=7.1-preview.4", url.PathEscape(project), url.PathEscape(id)))
}

func pullRequestsURL(org, project, repo, status, apiVersion string) string {
	return orgURL(org, fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests?searchCriteria.status=%s&api-version=%s", url.PathEscape(project), url.PathEscape(repo), url.QueryEscape(status), url.QueryEscape(apiVersion)))
}

func pullRequestURL(org, project, repo, id, apiVersion string) string {
	return orgURL(org, fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests/%s?api-version=%s", url.PathEscape(project), url.PathEscape(repo), url.PathEscape(id), url.QueryEscape(apiVersion)))
}

func pullRequestThreadsURL(org, project, repo, id, apiVersion string) string {
	return orgURL(org, fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests/%s/threads?api-version=%s", url.PathEscape(project), url.PathEscape(repo), url.PathEscape(id), url.QueryEscape(apiVersion)))
}

func wiqlURL(org, project, apiVersion string) string {
	return orgURL(org, fmt.Sprintf("/%s/_apis/wit/wiql?api-version=%s", url.PathEscape(project), url.QueryEscape(apiVersion)))
}

func codeSearchURL(org, apiVersion string) string {
	return "https://almsearch.dev.azure.com/" + url.PathEscape(org) + "/_apis/search/codesearchresults?api-version=" + url.QueryEscape(apiVersion)
}

func orgURL(org, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if strings.HasPrefix(org, "http://") || strings.HasPrefix(org, "https://") {
		return strings.TrimRight(org, "/") + "/" + strings.TrimLeft(path, "/")
	}
	return "https://dev.azure.com/" + url.PathEscape(org) + "/" + strings.TrimLeft(path, "/")
}

func readCacheBody(cachePath string, noCache bool) ([]byte, bool) {
	if noCache || cachePath == "" {
		return nil, false
	}
	body, err := os.ReadFile(cachePath)
	return body, err == nil
}

func cacheMetadataPath(cachePath string) string {
	ext := filepath.Ext(cachePath)
	if ext == "" {
		return cachePath + ".meta.json"
	}
	return strings.TrimSuffix(cachePath, ext) + ".meta.json"
}

func readCacheMetadata(cachePath string) cacheMetadata {
	var meta cacheMetadata
	body, err := os.ReadFile(cacheMetadataPath(cachePath))
	if err != nil {
		return meta
	}
	_ = json.Unmarshal(body, &meta)
	return meta
}

func writeCacheMetadata(cachePath string, meta cacheMetadata) error {
	return writeJSON(cacheMetadataPath(cachePath), meta)
}

func metadataFromResponse(rawURL string, res *http.Response, size int64, now time.Time) cacheMetadata {
	return cacheMetadata{
		URL:          rawURL,
		FetchedAt:    now,
		ValidatedAt:  now,
		ETag:         res.Header.Get("ETag"),
		LastModified: res.Header.Get("Last-Modified"),
		StatusCode:   res.StatusCode,
		Size:         size,
	}
}

func updateValidatorsFromResponse(meta *cacheMetadata, res *http.Response) {
	if etag := res.Header.Get("ETag"); etag != "" {
		meta.ETag = etag
	}
	if lastModified := res.Header.Get("Last-Modified"); lastModified != "" {
		meta.LastModified = lastModified
	}
}

func (m cacheMetadata) applyValidators(req *http.Request) {
	if m.ETag != "" {
		req.Header.Set("If-None-Match", m.ETag)
	}
	if m.LastModified != "" {
		req.Header.Set("If-Modified-Since", m.LastModified)
	}
}

func (c *adoClient) staleJSONResult(rawURL, cachePath string, body []byte, statusCode int, refreshErr error) cachedJSONResult {
	warning := fmt.Sprintf("using stale cached %s because refresh failed: %v", filepath.Base(cachePath), refreshErr)
	updateStaleCacheMetadata(rawURL, cachePath, statusCode, int64(len(body)), warning)
	return cachedJSONResult{Body: json.RawMessage(body), Warning: warning}
}

func staleDownloadResult(rawURL, cachePath string, statusCode int, refreshErr error) cachedDownloadResult {
	warning := fmt.Sprintf("using stale cached %s because refresh failed: %v", filepath.Base(cachePath), refreshErr)
	updateStaleCacheMetadata(rawURL, cachePath, statusCode, cacheFileSize(cachePath), warning)
	return cachedDownloadResult{Status: "cached-stale", Warning: warning}
}

func updateStaleCacheMetadata(rawURL, cachePath string, statusCode int, size int64, warning string) {
	meta := readCacheMetadata(cachePath)
	meta.URL = firstNonEmpty(meta.URL, rawURL)
	if meta.FetchedAt.IsZero() {
		meta.FetchedAt = cacheFileModTime(cachePath)
	}
	meta.ValidatedAt = time.Now().UTC()
	meta.StatusCode = statusCode
	meta.Size = size
	meta.Warning = warning
	_ = writeCacheMetadata(cachePath, meta)
}

func cacheFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func cacheFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func cacheFileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime().UTC()
}
