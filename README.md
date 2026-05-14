# ado-query

`ado-query` is a read-only Azure DevOps fetch CLI for agents. It fetches work items, comments, links, optional attachments, and small work-item trees, then writes files to disk.

It writes simple files:

- `content.md`
- `content.json`
- `raw/work-item.json`
- `raw/comments.json`
- optional `attachments/<guid>__<filename>` and `.md` conversions

There is no shared context library, no manifest, and no on-disk schema contract beyond the JSON files themselves.

## Install

```bash
go install github.com/charlesnpx/ado-query/cmd/ado-query@latest
```

For delegated installers:

```bash
./install-skill.sh --plan --target all --json
./install-skill.sh --install --target all --json
```

## Usage

```bash
az login
export ADO_ORG=ExampleOrg
export ADO_PROJECT=ExampleProject
mise-en-place setup ado-query --capability query

ado-query work-item 12345
ado-query work-item 12345 --include-attachments
ado-query work-item-tree 12345 --max-depth 2 --max-items 25
```

Common flags:

- `--org <org>` defaults to `$ADO_ORG`
- `--project <project>` defaults to `$ADO_PROJECT`
- `--out <dir>` defaults to `./.ado-query/<id>`
- `--cache-dir <dir>` defaults to `~/.cache/ado-query`
- `--no-cache` bypasses cache reads and writes
- `--api-version <ver>` defaults to `7.1`
- `--include-attachments` downloads attachments and tries `markitdown`
- `--max-attachment-bytes <n>` defaults to `25000000`

`work-item-tree` also accepts `--max-depth` and `--max-items`.

Auth uses the current Azure CLI Microsoft Entra login. The CLI fetches an Azure DevOps bearer token at runtime with:

```bash
az account get-access-token --resource 499b84ac-1321-427f-aa17-267ca6975798 --query accessToken -o tsv
```

`markitdown` is invoked as a subprocess. If it is missing, HTML fields are preserved in fenced `html` blocks and attachment conversion is skipped with warnings.
