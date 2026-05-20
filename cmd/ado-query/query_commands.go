package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

var queryValueFlags = map[string]bool{
	"api-version":          true,
	"max-attachment-bytes": true,
	"org":                  true,
	"project":              true,
	"top":                  true,
}

func runComments(args []string) error {
	opts, positionals, err := parseStandardQueryFlags("comments", args, true)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("comments requires exactly one work item id")
	}
	opts.id = positionals[0]
	body, err := fetchComments(context.Background(), opts)
	if err != nil {
		return err
	}
	return writeRawJSONStdout(body)
}

func runPRList(args []string) error {
	opts, positionals, err := parseStandardQueryFlags("pr-list", args, true)
	if err != nil {
		return err
	}
	if len(positionals) < 1 || len(positionals) > 2 {
		return errors.New("pr-list requires <repo> [status]")
	}
	status := "active"
	if len(positionals) == 2 {
		status = positionals[1]
	}
	body, err := fetchPullRequests(context.Background(), opts, positionals[0], status)
	if err != nil {
		return err
	}
	return writeRawJSONStdout(body)
}

func runPRGet(args []string) error {
	opts, positionals, err := parseStandardQueryFlags("pr-get", args, true)
	if err != nil {
		return err
	}
	if len(positionals) != 2 {
		return errors.New("pr-get requires <repo> <pr-id>")
	}
	body, err := fetchPullRequest(context.Background(), opts, positionals[0], positionals[1])
	if err != nil {
		return err
	}
	return writeRawJSONStdout(body)
}

func runPRThreads(args []string) error {
	opts, positionals, err := parseStandardQueryFlags("pr-threads", args, true)
	if err != nil {
		return err
	}
	if len(positionals) != 2 {
		return errors.New("pr-threads requires <repo> <pr-id>")
	}
	body, err := fetchPullRequestThreads(context.Background(), opts, positionals[0], positionals[1])
	if err != nil {
		return err
	}
	return writeRawJSONStdout(body)
}

func runWIQL(args []string) error {
	opts, positionals, err := parseStandardQueryFlags("wiql", args, true)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("wiql requires <query-or-@file>")
	}
	query, err := wiqlQueryText(positionals[0])
	if err != nil {
		return err
	}
	body, err := fetchWIQL(context.Background(), opts, query)
	if err != nil {
		return err
	}
	return writeRawJSONStdout(body)
}

func runSearchCode(args []string) error {
	opts := queryOptions{}
	top := 25
	fs := newStandardQueryFlagSet("search-code", &opts)
	fs.IntVar(&top, "top", top, "Maximum search results")
	positionals, err := parseFlagArgs(fs, args)
	if err != nil {
		return err
	}
	opts, err = normalizeQueryOptions("search-code", opts, true)
	if err != nil {
		return err
	}
	if top <= 0 {
		return errors.New("--top must be positive")
	}
	if len(positionals) != 1 {
		return errors.New("search-code requires exactly one search text argument")
	}
	body, err := fetchCodeSearch(context.Background(), opts, positionals[0], top)
	if err != nil {
		return err
	}
	return writeRawJSONStdout(body)
}

func runAPI(args []string) error {
	opts := queryOptions{}
	fs := newStandardQueryFlagSet("api", &opts)
	positionals, err := parseFlagArgs(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("api requires exactly one path or URL")
	}
	if opts.org == "" {
		opts.org = os.Getenv("ADO_ORG")
	}
	if opts.project == "" {
		opts.project = os.Getenv("ADO_PROJECT")
	}
	if opts.apiVersion == "" {
		opts.apiVersion = defaultAPIVersion
	}
	if opts.tokenProvider == nil {
		opts.tokenProvider = azureCLITokenProvider{resource: azureDevOpsResource}
	}
	if opts.org == "" && !isHTTPURL(positionals[0]) {
		return errors.New("api requires --org or ADO_ORG when using a relative path")
	}
	body, err := fetchAPI(context.Background(), opts, positionals[0])
	if err != nil {
		return err
	}
	return writeRawJSONStdout(body)
}

func runDownloadURL(args []string) error {
	opts := queryOptions{maxAttachmentBytes: defaultMaxAttachmentBytes}
	fs := flag.NewFlagSet("download-url", flag.ContinueOnError)
	fs.Int64Var(&opts.maxAttachmentBytes, "max-attachment-bytes", defaultMaxAttachmentBytes, "Maximum attachment bytes")
	positionals, err := parseFlagArgs(fs, args)
	if err != nil {
		return err
	}
	if opts.maxAttachmentBytes <= 0 {
		return errors.New("--max-attachment-bytes must be positive")
	}
	if len(positionals) != 2 {
		return errors.New("download-url requires <url> <output-path>")
	}
	opts.tokenProvider = azureCLITokenProvider{resource: azureDevOpsResource}
	return downloadURL(context.Background(), opts, positionals[0], positionals[1])
}

func parseStandardQueryFlags(name string, args []string, requireProject bool) (queryOptions, []string, error) {
	opts := queryOptions{}
	fs := newStandardQueryFlagSet(name, &opts)
	positionals, err := parseFlagArgs(fs, args)
	if err != nil {
		return opts, nil, err
	}
	opts, err = normalizeQueryOptions(name, opts, requireProject)
	if err != nil {
		return opts, nil, err
	}
	return opts, positionals, nil
}

func newStandardQueryFlagSet(name string, opts *queryOptions) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&opts.org, "org", "", "Azure DevOps organization")
	fs.StringVar(&opts.project, "project", "", "Azure DevOps project")
	fs.StringVar(&opts.apiVersion, "api-version", defaultAPIVersion, "Azure DevOps API version")
	return fs
}

func parseFlagArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	flagArgs, positionals, err := splitFlagArgs(args, queryValueFlags)
	if err != nil {
		return nil, err
	}
	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}
	return positionals, nil
}

func splitFlagArgs(args []string, valueFlags map[string]bool) ([]string, []string, error) {
	flags := []string{}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			name, _, hasInline := strings.Cut(name, "=")
			if valueFlags[name] && !hasInline {
				if i+1 >= len(args) {
					return nil, nil, fmt.Errorf("flag needs an argument: %s", arg)
				}
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return flags, positionals, nil
}

func normalizeQueryOptions(name string, opts queryOptions, requireProject bool) (queryOptions, error) {
	if opts.org == "" {
		opts.org = os.Getenv("ADO_ORG")
	}
	if opts.org == "" {
		return opts, fmt.Errorf("%s requires --org or ADO_ORG", name)
	}
	if opts.project == "" {
		opts.project = os.Getenv("ADO_PROJECT")
	}
	if requireProject && opts.project == "" {
		return opts, fmt.Errorf("%s requires --project or ADO_PROJECT", name)
	}
	if opts.tokenProvider == nil {
		opts.tokenProvider = azureCLITokenProvider{resource: azureDevOpsResource}
	}
	if opts.apiVersion == "" {
		opts.apiVersion = defaultAPIVersion
	}
	return opts, nil
}

func fetchComments(ctx context.Context, opts queryOptions) (json.RawMessage, error) {
	opts, err := normalizeQueryOptions("comments", opts, true)
	if err != nil {
		return nil, err
	}
	client := newADOClient(opts.tokenProvider)
	return client.fetchJSON(ctx, commentsURL(opts.org, opts.project, opts.id), "", true)
}

func fetchPullRequests(ctx context.Context, opts queryOptions, repo, status string) (json.RawMessage, error) {
	opts, err := normalizeQueryOptions("pr-list", opts, true)
	if err != nil {
		return nil, err
	}
	client := newADOClient(opts.tokenProvider)
	return client.fetchJSON(ctx, pullRequestsURL(opts.org, opts.project, repo, status, opts.apiVersion), "", true)
}

func fetchPullRequest(ctx context.Context, opts queryOptions, repo, id string) (json.RawMessage, error) {
	opts, err := normalizeQueryOptions("pr-get", opts, true)
	if err != nil {
		return nil, err
	}
	client := newADOClient(opts.tokenProvider)
	return client.fetchJSON(ctx, pullRequestURL(opts.org, opts.project, repo, id, opts.apiVersion), "", true)
}

func fetchPullRequestThreads(ctx context.Context, opts queryOptions, repo, id string) (json.RawMessage, error) {
	opts, err := normalizeQueryOptions("pr-threads", opts, true)
	if err != nil {
		return nil, err
	}
	client := newADOClient(opts.tokenProvider)
	return client.fetchJSON(ctx, pullRequestThreadsURL(opts.org, opts.project, repo, id, opts.apiVersion), "", true)
}

func fetchWIQL(ctx context.Context, opts queryOptions, query string) (json.RawMessage, error) {
	opts, err := normalizeQueryOptions("wiql", opts, true)
	if err != nil {
		return nil, err
	}
	client := newADOClient(opts.tokenProvider)
	return client.postJSON(ctx, wiqlURL(opts.org, opts.project, opts.apiVersion), map[string]string{"query": query})
}

func fetchCodeSearch(ctx context.Context, opts queryOptions, searchText string, top int) (json.RawMessage, error) {
	opts, err := normalizeQueryOptions("search-code", opts, true)
	if err != nil {
		return nil, err
	}
	if top <= 0 {
		return nil, errors.New("--top must be positive")
	}
	client := newADOClient(opts.tokenProvider)
	return client.postJSON(ctx, codeSearchURL(opts.org, opts.apiVersion), codeSearchBody(opts.project, searchText, top))
}

func codeSearchBody(project, searchText string, top int) map[string]any {
	return map[string]any{
		"searchText": searchText,
		"$top":       top,
		"filters": map[string][]string{
			"Project": []string{project},
		},
	}
}

func fetchAPI(ctx context.Context, opts queryOptions, path string) (json.RawMessage, error) {
	opts, err := normalizeQueryOptions("api", opts, false)
	if err != nil && !(opts.org == "" && isHTTPURL(path)) {
		return nil, err
	}
	client := newADOClient(opts.tokenProvider)
	return client.fetchJSON(ctx, orgURL(opts.org, path), "", true)
}

func downloadURL(ctx context.Context, opts queryOptions, rawURL, outputPath string) error {
	if opts.maxAttachmentBytes <= 0 {
		opts.maxAttachmentBytes = defaultMaxAttachmentBytes
	}
	client := newADOClient(opts.tokenProvider)
	return client.download(ctx, rawURL, outputPath, opts.maxAttachmentBytes)
}

func wiqlQueryText(input string) (string, error) {
	if !strings.HasPrefix(input, "@") {
		return input, nil
	}
	body, err := os.ReadFile(strings.TrimPrefix(input, "@"))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func writeRawJSONStdout(raw []byte) error {
	_, err := fmt.Println(strings.TrimSpace(string(raw)))
	return err
}

func isHTTPURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
