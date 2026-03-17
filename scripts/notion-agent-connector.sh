#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin_path="$repo_root/.tmp/bin/notion-agent-connector"

mkdir -p "$(dirname "$bin_path")"

needs_rebuild=false

if [[ ! -x "$bin_path" ]]; then
	needs_rebuild=true
fi

if [[ "$needs_rebuild" == false ]]; then
	for path in "$repo_root/go.mod" "$repo_root/go.sum"; do
		if [[ -e "$path" && "$path" -nt "$bin_path" ]]; then
			needs_rebuild=true
			break
		fi
	done
fi

if [[ "$needs_rebuild" == false ]] && find "$repo_root/cmd" "$repo_root/internal" -type f -name '*.go' -newer "$bin_path" -print -quit | grep -q .; then
	needs_rebuild=true
fi

if [[ "$needs_rebuild" == true ]]; then
	go build -o "$bin_path" "$repo_root/cmd/notion-agent-connector"
fi

exec "$bin_path" "$@"
