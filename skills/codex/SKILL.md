---
name: ado-query
description: "Use this skill for readonly Azure DevOps source research: work items, comments, links, and attachments."
---

# ado-query

Run `ado-query` when the user needs read-only Azure DevOps work item context.

```bash
ado-query work-item <id>
ado-query work-item-tree <id> --max-depth 2 --max-items 50
```

The CLI writes `content.md`, `content.json`, raw payloads, and optional attachments. Read `content.md` first, then inspect `content.json`, `raw/`, or `attachments/` when needed.

Use `--include-attachments` when attached artifacts matter. HTML fields and comments are converted through `markitdown` when available; otherwise the original HTML is preserved in fenced blocks.
