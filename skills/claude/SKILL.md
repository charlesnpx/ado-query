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
```

The CLI writes `content.md`, `content.json`, raw payloads, and optional attachment files to the output directory. Prefer `content.md` for normal context.

Auth comes from Azure CLI Microsoft Entra credentials, not an Azure DevOps PAT. Required setup: `az` on PATH, `az login`, and `ADO_ORG`. `ADO_PROJECT` is needed for comments.
