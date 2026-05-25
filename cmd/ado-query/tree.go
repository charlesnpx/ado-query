package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
)

type queueItem struct {
	id    string
	depth int
	path  []string
}

func fetchWorkItemTree(ctx context.Context, opts queryOptions) (treeContent, error) {
	var err error
	opts.tree = true
	opts, err = normalizeOptions(opts)
	if err != nil {
		return treeContent{}, err
	}
	if opts.maxItems <= 0 {
		opts.maxItems = defaultMaxTreeItems
	}
	rootOut := opts.outDir
	queue := []queueItem{{id: opts.id, depth: 0, path: []string{opts.id}}}
	seen := map[string]bool{}
	nodes := []treeNode{}
	edgesByKey := map[string]treeEdge{}
	warnings := []string{}
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if seen[item.id] {
			continue
		}
		if len(nodes) >= opts.maxItems {
			warnings = append(warnings, fmt.Sprintf("work item tree truncated at --max-items=%d", opts.maxItems))
			break
		}
		seen[item.id] = true
		itemOpts := opts
		itemOpts.id = item.id
		itemOpts.outDir = filepath.Join(rootOut, "items", item.id)
		content, err := fetchWorkItem(ctx, itemOpts)
		if err != nil {
			return treeContent{}, err
		}
		for _, warning := range content.Warnings {
			warnings = append(warnings, fmt.Sprintf("work item %s: %s", item.id, warning))
		}
		nodes = append(nodes, treeNode{
			ID: item.id, URL: content.URL, Depth: item.depth, Path: append([]string(nil), item.path...),
			Fields: content.Fields, Relations: content.Relations, ItemPath: filepath.ToSlash(filepath.Join("items", item.id)),
			CommentCount: len(content.Comments), AttachmentCount: len(content.Attachments),
		})
		for _, childID := range content.Relations.ChildIDs {
			key := item.id + "->" + childID
			edgesByKey[key] = treeEdge{FromID: item.id, ToID: childID, Rel: "System.LinkTypes.Hierarchy-Forward"}
			if item.depth < opts.maxDepth && !seen[childID] {
				queue = append(queue, queueItem{id: childID, depth: item.depth + 1, path: append(append([]string(nil), item.path...), childID)})
			}
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Depth != nodes[j].Depth {
			return nodes[i].Depth < nodes[j].Depth
		}
		return nodes[i].ID < nodes[j].ID
	})
	edges := make([]treeEdge, 0, len(edgesByKey))
	for _, edge := range edgesByKey {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].FromID != edges[j].FromID {
			return edges[i].FromID < edges[j].FromID
		}
		return edges[i].ToID < edges[j].ToID
	})
	content := treeContent{
		RootID: opts.id, URL: workItemURL(opts.org, opts.id, opts.apiVersion), Org: opts.org, Project: opts.project,
		MaxDepth: opts.maxDepth, MaxItems: opts.maxItems, Nodes: nodes, Edges: edges, Warnings: warnings,
	}
	if err := writeTreeOutput(rootOut, content); err != nil {
		return treeContent{}, err
	}
	return content, nil
}
