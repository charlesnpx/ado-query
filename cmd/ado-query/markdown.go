package main

import ("bytes"; "fmt"; "os"; "os/exec"; "path/filepath"; "strings")

var markitdownable = set(".pdf", ".docx", ".xlsx", ".pptx", ".html", ".htm", ".txt", ".md", ".csv", ".json", ".xml", ".rtf")
func markdownify(srcPath string) (string, error) { bin, err := exec.LookPath("markitdown"); if err != nil { return "", err }; cmd := exec.Command(bin, srcPath); var out, errBuf bytes.Buffer; cmd.Stdout = &out; cmd.Stderr = &errBuf; if err := cmd.Run(); err != nil { return "", fmt.Errorf("markitdown %s: %w (%s)", srcPath, err, strings.TrimSpace(errBuf.String())) }; return out.String(), nil }
func htmlToMarkdown(label, html string, warnings *[]string) string {
	if strings.TrimSpace(html) == "" { return "" }; tmp, err := os.CreateTemp("", "ado-query-*.html"); if err != nil { *warnings = append(*warnings, "failed to create temp file for "+label+": "+err.Error()); return fencedHTML(html) }
	path := tmp.Name(); _, writeErr := tmp.WriteString(html); closeErr := tmp.Close(); defer os.Remove(path); if writeErr != nil || closeErr != nil { *warnings = append(*warnings, "failed to write temp HTML for "+label); return fencedHTML(html) }
	md, err := markdownify(path); if err != nil { *warnings = append(*warnings, "markitdown unavailable for "+label+"; preserved raw HTML"); return fencedHTML(html) }; if strings.TrimSpace(md) == "" { return fencedHTML(html) }; return strings.TrimSpace(md)
}
func convertAttachment(path string, warnings *[]string) string { if !markitdownable[strings.ToLower(filepath.Ext(path))] { return "" }; md, err := markdownify(path); if err != nil { *warnings = append(*warnings, "markitdown skipped for "+filepath.Base(path)+": "+err.Error()); return "" }; outPath := path + ".md"; if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil { *warnings = append(*warnings, "failed to write "+filepath.Base(outPath)+": "+err.Error()); return "" }; return outPath }
func fencedHTML(html string) string { return "```html\n" + strings.TrimSpace(html) + "\n```" }
