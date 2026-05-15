package main

import "time"

const (
	defaultAPIVersion         = "7.1"
	defaultMaxAttachmentBytes = int64(25_000_000)
	defaultMaxTreeDepth       = 3
	defaultMaxTreeItems       = 50
)

type queryOptions struct {
	id, org, project, outDir, cacheDir, apiVersion string
	noCache, includeAttachments                    bool
	tree                                           bool
	maxAttachmentBytes                             int64
	maxDepth, maxItems                             int
	tokenProvider                                  tokenProvider
}
type fields struct {
	Title              string `json:"title"`
	State              string `json:"state"`
	AssignedTo         string `json:"assignedTo"`
	Iteration          string `json:"iteration"`
	Area               string `json:"area"`
	Tags               string `json:"tags"`
	Description        string `json:"description"`
	AcceptanceCriteria string `json:"acceptanceCriteria"`
}
type relations struct {
	ParentIDs  []string `json:"parentIds,omitempty"`
	ChildIDs   []string `json:"childIds,omitempty"`
	RelatedIDs []string `json:"relatedIds,omitempty"`
}
type comment struct {
	ID          any    `json:"id"`
	CreatedDate string `json:"createdDate"`
	CreatedBy   string `json:"createdBy"`
	Text        string `json:"text"`
}
type attachment struct {
	GUID             string   `json:"guid"`
	URL              string   `json:"url"`
	OriginalFilename string   `json:"originalFilename"`
	Extension        string   `json:"extension"`
	Sources          []string `json:"sources"`
	AssetPath        string   `json:"assetPath,omitempty"`
	MarkdownPath     string   `json:"markdownPath,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}
type workItemContent struct {
	ID          string       `json:"id"`
	URL         string       `json:"url"`
	Org         string       `json:"org"`
	Project     string       `json:"project,omitempty"`
	FetchedAt   time.Time    `json:"fetchedAt"`
	Fields      fields       `json:"fields"`
	Relations   relations    `json:"relations,omitempty"`
	Comments    []comment    `json:"comments"`
	Attachments []attachment `json:"attachments"`
	Warnings    []string     `json:"warnings,omitempty"`
}
type treeContent struct {
	RootID   string     `json:"rootId"`
	URL      string     `json:"url"`
	Org      string     `json:"org"`
	Project  string     `json:"project,omitempty"`
	MaxDepth int        `json:"maxDepth"`
	MaxItems int        `json:"maxItems"`
	Nodes    []treeNode `json:"nodes"`
	Edges    []treeEdge `json:"edges"`
	Warnings []string   `json:"warnings,omitempty"`
}
type treeNode struct {
	ID              string    `json:"id"`
	URL             string    `json:"url"`
	Depth           int       `json:"depth"`
	Path            []string  `json:"path"`
	Fields          fields    `json:"fields"`
	Relations       relations `json:"relations,omitempty"`
	ItemPath        string    `json:"itemPath"`
	CommentCount    int       `json:"commentCount"`
	AttachmentCount int       `json:"attachmentCount"`
}
type treeEdge struct {
	FromID string `json:"fromId"`
	ToID   string `json:"toId"`
	Rel    string `json:"rel"`
}
