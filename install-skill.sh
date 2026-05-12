#!/usr/bin/env bash
set -euo pipefail

NAME="ado-query"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSION="${ADO_QUERY_VERSION:-$(git -C "$repo_root" describe --tags --exact-match 2>/dev/null || true)}"
VERSION="${VERSION:-dev}"
OPERATION="install"
TARGET="all"
JSON="false"
INSTALL_ROOT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --plan) OPERATION="plan"; shift ;;
    --install) OPERATION="install"; shift ;;
    --uninstall) OPERATION="uninstall"; shift ;;
    --target) TARGET="${2:?missing --target value}"; shift 2 ;;
    --json) JSON="true"; shift ;;
    --install-root) INSTALL_ROOT="${2:?missing --install-root value}"; shift 2 ;;
    *) printf 'error: unknown argument: %s\n' "$1" >&2; exit 1 ;;
  esac
done

if [[ "$JSON" != "true" ]]; then
  printf 'error: this installer requires --json\n' >&2
  exit 1
fi
case "$TARGET" in all|codex|claude|tools) ;; *) printf 'error: unsupported target: %s\n' "$TARGET" >&2; exit 1 ;; esac

home_root="${INSTALL_ROOT:-$HOME}"
bin_path="$home_root/.local/bin/$NAME"
codex_path="$home_root/.codex/skills/$NAME/SKILL.md"
codex_agent_path="$home_root/.codex/skills/$NAME/agents/openai.yaml"
claude_path="$home_root/.claude/skills/$NAME/SKILL.md"
claude_agent_path="$home_root/.claude/agents/$NAME.md"

include_tools=false; include_codex=false; include_claude=false
case "$TARGET" in
  all) include_tools=true; include_codex=true; include_claude=true ;;
  codex) include_tools=true; include_codex=true ;;
  claude) include_tools=true; include_claude=true ;;
  tools) include_tools=true ;;
esac

sha_file() { if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'; else sha256sum "$1" | awk '{print $1}'; fi; }
json_escape() { local value="$1"; value="${value//\\/\\\\}"; value="${value//\"/\\\"}"; value="${value//$'\n'/\\n}"; printf '%s' "$value"; }

install_files() {
  if [[ "$include_tools" == true ]]; then mkdir -p "$(dirname "$bin_path")"; go build -ldflags "-X main.version=$VERSION" -o "$bin_path" "$repo_root/cmd/$NAME"; fi
  if [[ "$include_codex" == true ]]; then mkdir -p "$(dirname "$codex_path")" "$(dirname "$codex_agent_path")"; install -m 0644 "$repo_root/skills/codex/SKILL.md" "$codex_path"; install -m 0644 "$repo_root/skills/codex/agents/openai.yaml" "$codex_agent_path"; fi
  if [[ "$include_claude" == true ]]; then mkdir -p "$(dirname "$claude_path")" "$(dirname "$claude_agent_path")"; install -m 0644 "$repo_root/skills/claude/SKILL.md" "$claude_path"; install -m 0644 "$repo_root/agents/$NAME.md" "$claude_agent_path"; fi
}

if [[ "$OPERATION" == "install" ]]; then install_files; elif [[ "$OPERATION" == "uninstall" ]]; then
  [[ "$include_tools" == true ]] && rm -f "$bin_path"
  [[ "$include_codex" == true ]] && rm -f "$codex_path" "$codex_agent_path"
  [[ "$include_claude" == true ]] && rm -f "$claude_path" "$claude_agent_path"
fi

print_file() { local path="$1"; printf '{"path":"%s"' "$(json_escape "$path")"; if [[ "$OPERATION" == "install" && -f "$path" ]]; then printf ',"sha256":"%s"' "$(sha_file "$path")"; fi; printf '}'; }

first_target=true
printf '{"schema":1,"name":"%s","version":"%s","operation":"%s","kind":"delegated","capabilities":["query"],"setup":[{"kind":"executable","executable":"az","required_for":["query"],"remediation":"Install Azure CLI, then run az login."},{"kind":"azure-cli-auth","required_for":["query"],"remediation":"Run az login, then verify with az account show."},{"kind":"azure-devops-token","resource":"499b84ac-1321-427f-aa17-267ca6975798","required_for":["query"],"remediation":"Run az login with an account that can access Azure DevOps."},{"kind":"env","env":"ADO_ORG","value_class":"plain","required_for":["query"],"remediation":"Export ADO_ORG to the Azure DevOps organization name."}],"targets":{' "$NAME" "$VERSION" "$OPERATION"
emit_single() { local target_name="$1"; local path="$2"; [[ "$first_target" == false ]] && printf ','; first_target=false; printf '"%s":{"files":[' "$target_name"; print_file "$path"; printf ']}'; }
if [[ "$include_tools" == true ]]; then emit_single "tools" "$bin_path"; fi
if [[ "$include_codex" == true ]]; then [[ "$first_target" == false ]] && printf ','; first_target=false; printf '"codex":{"files":['; print_file "$codex_path"; printf ','; print_file "$codex_agent_path"; printf ']}'; fi
if [[ "$include_claude" == true ]]; then [[ "$first_target" == false ]] && printf ','; first_target=false; printf '"claude":{"files":['; print_file "$claude_path"; printf ','; print_file "$claude_agent_path"; printf ']}'; fi
printf '},"warnings":[]}\n'
