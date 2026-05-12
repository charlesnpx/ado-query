package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	attachmentRE = regexp.MustCompile(`(?i)/_apis/wit/attachments/([^/?#]+)`)
	workItemRE   = regexp.MustCompile(`(?i)/(?:_apis/wit/workitems|_workitems/edit)/([^/?#]+)`)
	urlAttrRE    = regexp.MustCompile(`(?i)(?:src|href)=["']([^"']+)["']`)
)

type discoveredAttachment struct {
	GUID             string
	URL              string
	OriginalFilename string
	Extension        string
	Sources          []string
}

func fetchWorkItem(ctx context.Context, opts queryOptions) (workItemContent, error) {
	var err error
	opts, err = normalizeOptions(opts)
	if err != nil {
		return workItemContent{}, err
	}
	client := newADOClient(opts.tokenProvider)
	cacheBase := filepath.Join(opts.cacheDir, safePath(opts.org), safePath(opts.project))
	itemURL := workItemURL(opts.org, opts.id, opts.apiVersion)
	rawItem, err := client.fetchJSON(ctx, itemURL, filepath.Join(cacheBase, "work-item-"+opts.id+".json"), opts.noCache)
	if err != nil {
		return workItemContent{}, err
	}
	var workItem map[string]any
	if err := json.Unmarshal(rawItem, &workItem); err != nil {
		return workItemContent{}, err
	}

	warnings := []string{}
	rawComments := json.RawMessage{}
	comments := map[string]any{}
	if opts.project != "" {
		rawComments, err = client.fetchJSON(ctx, commentsURL(opts.org, opts.project, opts.id), filepath.Join(cacheBase, "comments-"+opts.id+".json"), opts.noCache)
		if err != nil {
			warnings = append(warnings, "failed to fetch comments: "+err.Error())
		} else if err := json.Unmarshal(rawComments, &comments); err != nil {
			return workItemContent{}, err
		}
	} else {
		warnings = append(warnings, "ADO project not set; comments were not fetched")
	}

	rawFields := asMap(workItem["fields"])
	content := workItemContent{
		ID:        opts.id,
		URL:       itemURL,
		Org:       opts.org,
		Project:   opts.project,
		FetchedAt: time.Now().UTC(),
		Fields:    normalizeFields(rawFields, &warnings),
		Relations: normalizeRelations(workItem),
		Comments:  normalizeComments(comments, &warnings),
		Warnings:  warnings,
	}
	discovered := discoverAttachments(workItem, comments)
	if opts.includeAttachments {
		for _, discovered := range discovered {
			content.Attachments = append(content.Attachments, materializeAttachment(ctx, client, opts, cacheBase, discovered))
		}
	} else {
		for _, discovered := range discovered {
			content.Attachments = append(content.Attachments, attachment{GUID: discovered.GUID, URL: discovered.URL, OriginalFilename: discovered.OriginalFilename, Extension: discovered.Extension, Sources: discovered.Sources})
		}
	}
	for _, att := range content.Attachments {
		content.Warnings = append(content.Warnings, att.Warnings...)
	}
	if err := writeWorkItemOutput(opts.outDir, content, rawItem, rawComments); err != nil {
		return workItemContent{}, err
	}
	return content, nil
}

func normalizeOptions(opts queryOptions) (queryOptions, error) {
	if opts.org == "" {
		opts.org = os.Getenv("ADO_ORG")
	}
	if opts.org == "" {
		return opts, fmt.Errorf("missing Azure DevOps org; set ADO_ORG or pass --org")
	}
	if opts.project == "" {
		opts.project = os.Getenv("ADO_PROJECT")
	}
	if opts.tokenProvider == nil {
		opts.tokenProvider = azureCLITokenProvider{resource: azureDevOpsResource}
	}
	if opts.apiVersion == "" {
		opts.apiVersion = defaultAPIVersion
	}
	if opts.cacheDir == "" {
		cacheDir, err := defaultCacheDir("ado-query")
		if err != nil {
			return opts, err
		}
		opts.cacheDir = cacheDir
	}
	if opts.outDir == "" {
		opts.outDir = filepath.Join(".ado-query", opts.id)
	}
	if opts.maxAttachmentBytes <= 0 {
		opts.maxAttachmentBytes = defaultMaxAttachmentBytes
	}
	return opts, nil
}

func normalizeFields(raw map[string]any, warnings *[]string) fields {
	return fields{
		Title:              stringValue(raw["System.Title"]),
		State:              stringValue(raw["System.State"]),
		AssignedTo:         identity(raw["System.AssignedTo"]),
		Iteration:          stringValue(raw["System.IterationPath"]),
		Area:               stringValue(raw["System.AreaPath"]),
		Tags:               stringValue(raw["System.Tags"]),
		Description:        htmlToMarkdown("description", stringValue(raw["System.Description"]), warnings),
		AcceptanceCriteria: htmlToMarkdown("acceptance criteria", stringValue(raw["Microsoft.VSTS.Common.AcceptanceCriteria"]), warnings),
	}
}

func normalizeComments(raw map[string]any, warnings *[]string) []comment {
	out := []comment{}
	for _, item := range asSlice(raw["comments"]) {
		c := asMap(item)
		out = append(out, comment{
			ID:          c["id"],
			CreatedDate: stringValue(c["createdDate"]),
			CreatedBy:   identity(c["createdBy"]),
			Text:        htmlToMarkdown("comment", stringValue(c["text"]), warnings),
		})
	}
	return out
}

func normalizeRelations(workItem map[string]any) relations {
	out := relations{}
	for _, relAny := range asSlice(workItem["relations"]) {
		rel := asMap(relAny)
		kind := stringValue(rel["rel"])
		id := workItemIDFromURL(stringValue(rel["url"]))
		if id == "" || kind == "AttachedFile" {
			continue
		}
		switch kind {
		case "System.LinkTypes.Hierarchy-Forward":
			out.ChildIDs = append(out.ChildIDs, id)
		case "System.LinkTypes.Hierarchy-Reverse":
			out.ParentIDs = append(out.ParentIDs, id)
		default:
			out.RelatedIDs = append(out.RelatedIDs, id)
		}
	}
	out.ParentIDs = sortedUnique(out.ParentIDs)
	out.ChildIDs = sortedUnique(out.ChildIDs)
	out.RelatedIDs = sortedUnique(out.RelatedIDs)
	return out
}

func discoverAttachments(workItem, comments map[string]any) []discoveredAttachment {
	byGUID := map[string]*discoveredAttachment{}
	fields := asMap(workItem["fields"])
	for source, body := range map[string]string{"description": stringValue(fields["System.Description"]), "acceptance_criteria": stringValue(fields["Microsoft.VSTS.Common.AcceptanceCriteria"])} {
		for _, match := range urlAttrRE.FindAllStringSubmatch(body, -1) {
			addAttachment(byGUID, match[1], source, "")
		}
	}
	for _, commentAny := range asSlice(comments["comments"]) {
		comment := asMap(commentAny)
		source := "comment:" + stringValue(comment["id"])
		for _, match := range urlAttrRE.FindAllStringSubmatch(stringValue(comment["text"]), -1) {
			addAttachment(byGUID, match[1], source, "")
		}
	}
	for _, relAny := range asSlice(workItem["relations"]) {
		rel := asMap(relAny)
		if stringValue(rel["rel"]) != "AttachedFile" {
			continue
		}
		addAttachment(byGUID, stringValue(rel["url"]), "attachment", stringValue(asMap(rel["attributes"])["name"]))
	}
	out := make([]discoveredAttachment, 0, len(byGUID))
	for _, value := range byGUID {
		out = append(out, *value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GUID < out[j].GUID })
	return out
}

func addAttachment(byGUID map[string]*discoveredAttachment, rawURL, source, filename string) {
	unescaped := html.UnescapeString(rawURL)
	match := attachmentRE.FindStringSubmatch(unescaped)
	if match == nil {
		return
	}
	guid, _ := url.PathUnescape(match[1])
	parsed, _ := url.Parse(unescaped)
	name := filename
	if name == "" && parsed != nil {
		name = firstNonEmpty(parsed.Query().Get("fileName"), parsed.Query().Get("filename"))
	}
	if name == "" {
		name = guid + ".bin"
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		ext = ".bin"
	}
	entry, ok := byGUID[guid]
	if !ok {
		entry = &discoveredAttachment{GUID: guid, URL: unescaped, OriginalFilename: name, Extension: ext}
		byGUID[guid] = entry
	}
	if !contains(entry.Sources, source) {
		entry.Sources = append(entry.Sources, source)
	}
}

func materializeAttachment(ctx context.Context, client *adoClient, opts queryOptions, cacheBase string, a discoveredAttachment) attachment {
	outName := safeFileName(a.GUID + "__" + a.OriginalFilename)
	outPath := filepath.Join(opts.outDir, "attachments", outName)
	cachePath := filepath.Join(cacheBase, "attachment-"+a.GUID+a.Extension)
	warnings := []string{}
	if opts.noCache {
		if err := client.download(ctx, a.URL, outPath, opts.maxAttachmentBytes); err != nil {
			warnings = append(warnings, "failed to download "+a.OriginalFilename+": "+err.Error())
		}
	} else {
		if _, err := os.Stat(cachePath); err != nil {
			if err := client.download(ctx, a.URL, cachePath, opts.maxAttachmentBytes); err != nil {
				warnings = append(warnings, "failed to download "+a.OriginalFilename+": "+err.Error())
			}
		}
		if len(warnings) == 0 {
			if err := copyFile(cachePath, outPath); err != nil {
				warnings = append(warnings, "failed to copy "+a.OriginalFilename+": "+err.Error())
			}
		}
	}
	mdRel := ""
	if len(warnings) == 0 {
		mdPath := ""
		cachedMD := filepath.Join(cacheBase, "attachment-"+a.GUID+".md")
		if !opts.noCache && copyFile(cachedMD, outPath+".md") == nil {
			mdPath = outPath + ".md"
		} else {
			mdPath = convertAttachment(outPath, &warnings)
		}
		if mdPath != "" {
			mdRel = relSlash(opts.outDir, mdPath)
			if !opts.noCache {
				_ = copyFile(mdPath, cachedMD)
			}
		}
	}
	return attachment{GUID: a.GUID, URL: a.URL, OriginalFilename: a.OriginalFilename, Extension: a.Extension, Sources: a.Sources, AssetPath: relIfExists(opts.outDir, outPath), MarkdownPath: mdRel, Warnings: warnings}
}
