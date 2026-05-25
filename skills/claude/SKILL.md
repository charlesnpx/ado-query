---
name: ado-query
description: "Read-only Azure DevOps retrieval for work items, comments, links, and attachments."
---

# ado-query

Use `ado-query` for read-only Azure DevOps context.

When available, check setup first:

```bash
mise-en-place setup ado-query --capability query
```

```bash
ado-query work-item <id>
ado-query work-item-tree <id> --max-depth 2
ado-query comments <id>
ado-query pr-list <repo> [status]
ado-query pr-get <repo> <pr-id>
ado-query pr-threads <repo> <pr-id>
ado-query wiql "<query>"
ado-query search-code "<text>"
ado-query api <path-or-url>
```

The CLI writes `content.md`, `content.json`, raw payloads, and optional attachment files to the output directory, which defaults to `~/.cache/ado-query/outputs/...`. Always run the CLI for current context instead of trusting an old output directory directly; `work-item` and `work-item-tree` validate cached work items, comments, and included attachments on every run, and they report stale or missing cache coverage in warnings. Prefer `content.md` for normal context. Pass `--out <dir>` only when a caller-local output directory is required.

The standalone query commands print raw JSON to stdout. Use them for focused reads such as comments, PR metadata, PR review threads, WIQL results, code search, or generic read-only API calls.

Auth comes from Azure CLI Microsoft Entra credentials, not an Azure DevOps PAT. Required setup: `az` on PATH, `az login`, and `ADO_ORG`. `ADO_PROJECT` is needed for comments.

Use `--include-attachments` when attached artifacts matter. Check attachment status and warnings in `content.md` or `content.json`; missing attachments are retried on later runs.
