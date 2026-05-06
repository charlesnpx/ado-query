package main

import ("encoding/json"; "fmt"; "os"; "path/filepath"; "sort"; "strings")

func writeWorkItemOutput(outDir string, content workItemContent, rawWorkItem, rawComments json.RawMessage) error {
	if err := writeRawJSON(filepath.Join(outDir, "raw", "work-item.json"), rawWorkItem); err != nil { return err }; if len(rawComments) > 0 { if err := writeRawJSON(filepath.Join(outDir, "raw", "comments.json"), rawComments); err != nil { return err } }
	if err := writeJSON(filepath.Join(outDir, "content.json"), content); err != nil { return err }; return writeWorkItemMarkdown(filepath.Join(outDir, "content.md"), content)
}
func writeWorkItemMarkdown(path string, c workItemContent) error {
	var b strings.Builder; fmt.Fprintf(&b, "# ADO Work Item %s: %s\n\n", c.ID, firstNonEmpty(c.Fields.Title, "(untitled)")); fmt.Fprintf(&b, "- State: %s\n- Assigned To: %s\n- Iteration: %s\n- Area: %s\n- Tags: %s\n\n", c.Fields.State, c.Fields.AssignedTo, c.Fields.Iteration, c.Fields.Area, c.Fields.Tags)
	b.WriteString("## Description\n\n" + firstNonEmpty(c.Fields.Description, "(empty)") + "\n\n## Acceptance Criteria\n\n" + firstNonEmpty(c.Fields.AcceptanceCriteria, "(empty)") + "\n\n## Comments\n\n"); if len(c.Comments) == 0 { b.WriteString("No comments fetched.\n") } else { for _, cm := range c.Comments { fmt.Fprintf(&b, "- %s by %s\n\n%s\n\n", cm.CreatedDate, firstNonEmpty(cm.CreatedBy, "(unknown)"), firstNonEmpty(cm.Text, "(empty)")) } }
	fmt.Fprintf(&b, "## Links\n\n- Parents: %s\n- Children: %s\n- Related: %s\n\n## Attachments\n\n", strings.Join(c.Relations.ParentIDs, ", "), strings.Join(c.Relations.ChildIDs, ", "), strings.Join(c.Relations.RelatedIDs, ", ")); if len(c.Attachments) == 0 { b.WriteString("No attachments.\n") } else { for _, att := range c.Attachments { fmt.Fprintf(&b, "- %s", att.OriginalFilename); if att.AssetPath != "" { fmt.Fprintf(&b, " `%s`", att.AssetPath) }; if att.MarkdownPath != "" { fmt.Fprintf(&b, " markdown `%s`", att.MarkdownPath) }; b.WriteByte('\n') } }
	if len(c.Warnings) > 0 { b.WriteString("\n## Warnings\n\n"); for _, warning := range c.Warnings { fmt.Fprintf(&b, "- %s\n", warning) } }; return writeFile(path, []byte(b.String()))
}
func writeTreeOutput(outDir string, content treeContent) error {
	if err := writeJSON(filepath.Join(outDir, "content.json"), content); err != nil { return err }; var b strings.Builder; fmt.Fprintf(&b, "# ADO Work Item Tree %s\n\n- Nodes fetched: %d\n- Edges discovered: %d\n- Max depth: %d\n- Max items: %d\n\n", content.RootID, len(content.Nodes), len(content.Edges), content.MaxDepth, content.MaxItems)
	for _, n := range content.Nodes { fmt.Fprintf(&b, "- %s depth %d: %s [%s] `%s`\n", n.ID, n.Depth, firstNonEmpty(n.Fields.Title, "(untitled)"), n.Fields.State, n.ItemPath) }; if len(content.Edges) > 0 { b.WriteString("\n## Hierarchy\n\n"); for _, e := range content.Edges { fmt.Fprintf(&b, "- %s -> %s\n", e.FromID, e.ToID) } }
	if len(content.Warnings) > 0 { b.WriteString("\n## Warnings\n\n"); for _, warning := range content.Warnings { fmt.Fprintf(&b, "- %s\n", warning) } }; return writeFile(filepath.Join(outDir, "content.md"), []byte(b.String()))
}
func writeJSON(path string, value any) error { body, err := json.MarshalIndent(value, "", "  "); if err != nil { return err }; return writeFile(path, append(body, '\n')) }
func writeRawJSON(path string, raw []byte) error { return writeFile(path, append([]byte(strings.TrimSpace(string(raw))), '\n')) }
func writeFile(path string, body []byte) error { if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }; return os.WriteFile(path, body, 0o644) }
func sortedUnique(in []string) []string { seen := map[string]bool{}; out := []string{}; for _, item := range in { if item != "" && !seen[item] { seen[item] = true; out = append(out, item) } }; sort.Strings(out); return out }
