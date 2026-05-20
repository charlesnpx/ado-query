---
name: ado-query
description: "Use this skill for readonly Azure DevOps source research: work items, comments, links, and attachments."
---

# ado-query

Run `ado-query` when the user needs read-only Azure DevOps work item context.

When available, check setup first:

```bash
mise-en-place setup ado-query --capability query
```

```bash
ado-query work-item <id>
ado-query work-item-tree <id> --max-depth 2 --max-items 50
ado-query comments <id>
ado-query pr-list <repo> [status]
ado-query pr-get <repo> <pr-id>
ado-query pr-threads <repo> <pr-id>
ado-query wiql "<query>"
ado-query search-code "<text>"
ado-query api <path-or-url>
```

The CLI writes `content.md`, `content.json`, raw payloads, and optional attachments under its output directory, which defaults to `~/.cache/ado-query/outputs/...`. Read `content.md` first, then inspect `content.json`, `raw/`, or `attachments/` when needed. Pass `--out <dir>` only when a caller-local output directory is required.

The standalone query commands print raw JSON to stdout. Use them when you need a narrow read such as comments, PR metadata, PR review threads, WIQL results, code search, or an arbitrary read-only API endpoint.

Auth comes from Azure CLI Microsoft Entra credentials, not an Azure DevOps PAT. The user must be logged in with `az login`; `ADO_ORG` supplies the organization, and `ADO_PROJECT` enables comments.

Use `--include-attachments` when attached artifacts matter. HTML fields and comments are converted through `markitdown` when available; otherwise the original HTML is preserved in fenced blocks.
