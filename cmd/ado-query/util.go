package main

import ("fmt"; "io"; "os"; "os/exec"; "path/filepath"; "regexp"; "strings")

func defaultCacheDir(name string) (string, error) { home, err := os.UserHomeDir(); if err != nil { return "", err }; return filepath.Join(home, name+"-cache"), nil }
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { return err }; in, err := os.Open(src); if err != nil { return err }; defer in.Close()
	tmp := dst + ".tmp"; out, err := os.Create(tmp); if err != nil { return err }; _, copyErr := io.Copy(out, in); closeErr := out.Close()
	if copyErr != nil { _ = os.Remove(tmp); return copyErr }; if closeErr != nil { _ = os.Remove(tmp); return closeErr }; return os.Rename(tmp, dst)
}
func workItemIDFromURL(rawURL string) string { match := workItemRE.FindStringSubmatch(rawURL); if match == nil { return "" }; return match[1] }
func relIfExists(base, path string) string { if info, err := os.Stat(path); err == nil && !info.IsDir() { return relSlash(base, path) }; return "" }
func relSlash(base, path string) string { rel, err := filepath.Rel(base, path); if err != nil { return filepath.ToSlash(path) }; return filepath.ToSlash(rel) }
func safePath(value string) string { if value == "" { return "_" }; return strings.Trim(regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(value, "_"), "_") }
func safeFileName(value string) string { return strings.TrimSpace(regexp.MustCompile(`[^A-Za-z0-9._ -]+`).ReplaceAllString(filepath.Base(value), "_")) }
func identity(value any) string { if m := asMap(value); len(m) > 0 { return firstNonEmpty(stringValue(m["displayName"]), stringValue(m["uniqueName"])) }; return stringValue(value) }
func stringValue(value any) string { switch v := value.(type) { case nil: return ""; case string: return v; case float64: if v == float64(int64(v)) { return fmt.Sprintf("%d", int64(v)) }; return fmt.Sprintf("%v", v); default: return fmt.Sprintf("%v", v) } }
func asMap(value any) map[string]any { if v, ok := value.(map[string]any); ok { return v }; return map[string]any{} }
func asSlice(value any) []any { if v, ok := value.([]any); ok { return v }; return nil }
func contains(values []string, want string) bool { for _, value := range values { if value == want { return true } }; return false }
func firstNonEmpty(values ...string) string { for _, value := range values { if value != "" { return value } }; return "" }
func set(values ...string) map[string]bool { out := map[string]bool{}; for _, value := range values { out[value] = true }; return out }
func installerPath() (string, error) { for _, path := range []string{filepath.Join(mustGetwd(), "install-skill.sh"), gitRootInstaller()} { if info, err := os.Stat(path); err == nil && !info.IsDir() { return path, nil } }; return "", fmt.Errorf("install-skill.sh not found") }
func mustGetwd() string { wd, _ := os.Getwd(); return wd }
func gitRootInstaller() string { out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); if err != nil { return "" }; return filepath.Join(strings.TrimSpace(string(out)), "install-skill.sh") }
