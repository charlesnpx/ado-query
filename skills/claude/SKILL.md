---
name: ado-query
description: "Read-only Azure DevOps retrieval for work items, comments, links, and attachments."
---

# ado-query

Use `ado-query` for read-only Azure DevOps context.

```bash
ado-query work-item <id>
ado-query work-item-tree <id> --max-depth 2
```

The CLI writes `content.md`, `content.json`, raw payloads, and optional attachment files to the output directory. Prefer `content.md` for normal context.

Required environment: `AZURE_DEVOPS_PAT` and `ADO_ORG`. `ADO_PROJECT` is needed for comments.
