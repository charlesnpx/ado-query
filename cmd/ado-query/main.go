package main

import ("context"; "errors"; "flag"; "fmt"; "os"; "os/exec"; "strings")

var version = "dev"
func main() { if err := run(os.Args[1:]); err != nil { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) } }
func run(args []string) error {
	if len(args) == 0 { usage(); return errors.New("missing command") }
	switch args[0] { case "work-item": return runWorkItem(args[1:]); case "work-item-tree": return runTree(args[1:]); case "install-skill": return runInstaller(args[1:]); case "--version", "-version", "version": fmt.Println(version); return nil; case "-h", "--help", "help": usage(); return nil; default: return fmt.Errorf("unknown command: %s", args[0]) }
}
func runWorkItem(args []string) error { opts, err := parseQueryFlags("work-item", args, false); if err != nil { return err }; _, err = fetchWorkItem(context.Background(), opts); if err == nil { fmt.Println(opts.outDir) }; return err }
func runTree(args []string) error { opts, err := parseQueryFlags("work-item-tree", args, true); if err != nil { return err }; _, err = fetchWorkItemTree(context.Background(), opts); if err == nil { fmt.Println(opts.outDir) }; return err }
func parseQueryFlags(name string, args []string, tree bool) (queryOptions, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError); opts := queryOptions{}; fs.StringVar(&opts.org, "org", "", "Azure DevOps organization"); fs.StringVar(&opts.project, "project", "", "Azure DevOps project"); fs.StringVar(&opts.outDir, "out", "", "Output directory"); fs.StringVar(&opts.cacheDir, "cache-dir", "", "Cache directory"); fs.BoolVar(&opts.noCache, "no-cache", false, "Bypass cache"); fs.StringVar(&opts.pat, "pat", "", "Azure DevOps PAT"); fs.StringVar(&opts.apiVersion, "api-version", defaultAPIVersion, "Azure DevOps API version"); fs.BoolVar(&opts.includeAttachments, "include-attachments", false, "Download attachments"); fs.Int64Var(&opts.maxAttachmentBytes, "max-attachment-bytes", defaultMaxAttachmentBytes, "Maximum attachment bytes")
	if tree { fs.IntVar(&opts.maxDepth, "max-depth", defaultMaxTreeDepth, "Maximum tree depth"); fs.IntVar(&opts.maxItems, "max-items", defaultMaxTreeItems, "Maximum work items") }
	flagArgs, id, err := splitArgs(args, map[string]bool{"org": true, "project": true, "out": true, "cache-dir": true, "pat": true, "api-version": true, "max-attachment-bytes": true, "max-depth": true, "max-items": true}); if err != nil { return opts, err }; if err := fs.Parse(flagArgs); err != nil { return opts, err }; if id == "" { return opts, fmt.Errorf("%s requires exactly one id", name) }; if opts.maxAttachmentBytes <= 0 { return opts, errors.New("--max-attachment-bytes must be positive") }; if tree && (opts.maxDepth < 0 || opts.maxItems <= 0) { return opts, errors.New("--max-depth must be non-negative and --max-items must be positive") }; opts.id = id; if opts.outDir == "" { opts.outDir = ".ado-query/" + id }; return opts, nil
}
func splitArgs(args []string, valueFlags map[string]bool) ([]string, string, error) { flags := []string{}; id := ""; for i := 0; i < len(args); i++ { arg := args[i]; if strings.HasPrefix(arg, "-") && arg != "-" { flags = append(flags, arg); name := strings.TrimLeft(arg, "-"); name, _, hasInline := strings.Cut(name, "="); if valueFlags[name] && !hasInline { if i+1 >= len(args) { return nil, "", fmt.Errorf("flag needs an argument: %s", arg) }; i++; flags = append(flags, args[i]) }; continue }; if id != "" { return nil, "", errors.New("expected exactly one id") }; id = arg }; return flags, id, nil }
func runInstaller(args []string) error { script, err := installerPath(); if err != nil { return err }; cmd := exec.Command(script, args...); cmd.Stdout = os.Stdout; cmd.Stderr = os.Stderr; cmd.Stdin = os.Stdin; return cmd.Run() }
func usage() { fmt.Println(`Usage:
  ado-query work-item <id> [flags]
  ado-query work-item-tree <id> [flags]
  ado-query install-skill [--plan|--install|--uninstall] [--target all|claude|codex|tools] [--json] [--install-root <dir>]
  ado-query --version | --help`) }
