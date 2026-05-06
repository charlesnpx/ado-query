package main

import ("context"; "encoding/base64"; "encoding/json"; "fmt"; "io"; "net/http"; "net/url"; "os"; "path/filepath"; "strings"; "time")

type adoClient struct { pat string; http *http.Client }
func newADOClient(pat string) adoClient { return adoClient{pat: pat, http: &http.Client{Timeout: 60 * time.Second}} }
func (c adoClient) fetchJSON(ctx context.Context, rawURL, cachePath string, noCache bool) (json.RawMessage, error) {
	if !noCache && cachePath != "" { if body, err := os.ReadFile(cachePath); err == nil { return json.RawMessage(body), nil } }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil); if err != nil { return nil, err }; req.Header.Set("Authorization", authHeader(c.pat)); req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req); if err != nil { return nil, err }; defer res.Body.Close(); body, err := io.ReadAll(res.Body); if err != nil { return nil, err }
	if res.StatusCode < 200 || res.StatusCode >= 300 { return nil, fmt.Errorf("GET %s: %s: %s", rawURL, res.Status, strings.TrimSpace(string(body))) }
	if !noCache && cachePath != "" { if err := writeRawJSON(cachePath, body); err != nil { return nil, err } }; return json.RawMessage(body), nil
}
func (c adoClient) download(ctx context.Context, rawURL, dst string, maxBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil); if err != nil { return err }; req.Header.Set("Authorization", authHeader(c.pat)); res, err := c.http.Do(req); if err != nil { return err }; defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 { return fmt.Errorf("download %s: %s", rawURL, res.Status) }; if res.ContentLength > maxBytes { return fmt.Errorf("attachment exceeds %d byte limit", maxBytes) }; if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { return err }
	tmp := dst + ".tmp"; out, err := os.Create(tmp); if err != nil { return err }; written, copyErr := io.Copy(out, &io.LimitedReader{R: res.Body, N: maxBytes + 1}); closeErr := out.Close()
	if copyErr != nil { _ = os.Remove(tmp); return copyErr }; if closeErr != nil { _ = os.Remove(tmp); return closeErr }; if written > maxBytes { _ = os.Remove(tmp); return fmt.Errorf("attachment exceeds %d byte limit", maxBytes) }; return os.Rename(tmp, dst)
}
func workItemURL(org, id, apiVersion string) string { return orgURL(org, fmt.Sprintf("/_apis/wit/workitems/%s?$expand=relations&api-version=%s", url.PathEscape(id), url.QueryEscape(apiVersion))) }
func commentsURL(org, project, id string) string { return orgURL(org, fmt.Sprintf("/%s/_apis/wit/workItems/%s/comments?api-version=7.1-preview.4", url.PathEscape(project), url.PathEscape(id))) }
func orgURL(org, path string) string { if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") { return path }; if strings.HasPrefix(org, "http://") || strings.HasPrefix(org, "https://") { return strings.TrimRight(org, "/") + "/" + strings.TrimLeft(path, "/") }; return "https://dev.azure.com/" + url.PathEscape(org) + "/" + strings.TrimLeft(path, "/") }
func authHeader(pat string) string { return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+pat)) }
