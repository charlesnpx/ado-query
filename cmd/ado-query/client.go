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

func newADOClient(provider tokenProvider) *adoClient {
	if provider == nil {
		provider = azureCLITokenProvider{resource: azureDevOpsResource}
	}
	return &adoClient{tokenProvider: provider, http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *adoClient) fetchJSON(ctx context.Context, rawURL, cachePath string, noCache bool) (json.RawMessage, error) {
	if !noCache && cachePath != "" {
		if body, err := os.ReadFile(cachePath); err == nil {
			return json.RawMessage(body), nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if err := c.authorize(ctx, req); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: %s: %s", rawURL, res.Status, strings.TrimSpace(string(body)))
	}
	if !noCache && cachePath != "" {
		if err := writeRawJSON(cachePath, body); err != nil {
			return nil, err
		}
	}
	return json.RawMessage(body), nil
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
