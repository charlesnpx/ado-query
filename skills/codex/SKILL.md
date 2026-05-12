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
```

The CLI writes `content.md`, `content.json`, raw payloads, and optional attachments. Read `content.md` first, then inspect `content.json`, `raw/`, or `attachments/` when needed.

Auth comes from Azure CLI Microsoft Entra credentials, not an Azure DevOps PAT. The user must be logged in with `az login`; `ADO_ORG` supplies the organization, and `ADO_PROJECT` enables comments.

Use `--include-attachments` when attached artifacts matter. HTML fields and comments are converted through `markitdown` when available; otherwise the original HTML is preserved in fenced blocks.
