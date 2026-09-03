#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
COMPOSE_UPSTREAM="$ROOT/.work/upstream/compose-unpacker"
PORTAINER_UPSTREAM="$ROOT/.work/upstream/portainer"

failures=()

while IFS= read -r path; do
	[[ -f "$ROOT/$path" ]] || continue
	lower_path=$(printf '%s' "$path" | tr '[:upper:]' '[:lower:]')
	case "$lower_path" in
		docs/superpowers/* | notes.md | */notes.md)
			failures+=("$path is a historical planning or notes asset")
			;;
	esac
done < <(git -C "$ROOT" ls-files)

for upstream in "$COMPOSE_UPSTREAM" "$PORTAINER_UPSTREAM"; do
	if [[ ! -d "$upstream/.git" ]]; then
		echo "prepared upstream tree is missing: $upstream" >&2
		exit 1
	fi

	while IFS= read -r path; do
		[[ -f "$ROOT/$path" ]] || continue
		blob=$(git -C "$ROOT" hash-object "$path")
		matches=$(
			git -C "$upstream" ls-tree -r HEAD |
				awk -v blob="$blob" '$3 == blob { print $4 }' |
				paste -sd ' ' -
		)
		if [[ -n "$matches" ]]; then
			failures+=("$path duplicates $(basename "$upstream") upstream: $matches")
		fi
	done < <(git -C "$ROOT" ls-files)
done

if ((${#failures[@]} > 0)); then
	printf 'repository contains assets that are not downstream-owned differences:\n' >&2
	printf '  - %s\n' "${failures[@]}" >&2
	exit 1
fi
