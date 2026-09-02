#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
SCRATCH="$ROOT/.tmp/test-update-workflow.$$"
FAKE_BIN="$SCRATCH/bin"
OUTPUT="$SCRATCH/output"
GITHUB_OUTPUT_FILE="$SCRATCH/github-output"
STEP_SCRIPT="$SCRATCH/resolve-updates.sh"

cleanup() {
	rm -rf "$SCRATCH"
}
trap cleanup EXIT

fail() {
	cat "$OUTPUT" >&2 2>/dev/null || true
	echo "$*" >&2
	exit 1
}

rm -rf "$SCRATCH"
mkdir -p "$FAKE_BIN"

ruby -rpsych -e '
  workflow = Psych.safe_load(File.read(ARGV.fetch(0)), aliases: true)
  step = workflow.fetch("jobs").fetch("update").fetch("steps")
    .find { |candidate| candidate["name"] == "Resolve updates" }
  abort "Resolve updates step not found" unless step
  print step.fetch("run")
' "$ROOT/.github/workflows/update.yml" >"$STEP_SCRIPT"

cat >"$FAKE_BIN/go" <<'EOF'
#!/bin/sh
printf '{"portainer":{"from":"2.45.0","to":"2.46.0"}}\n'
EOF

cat >"$FAKE_BIN/git" <<'EOF'
#!/bin/sh
if [ "$*" = "diff --quiet -- versions.json" ]; then
	exit 2
fi
echo "unexpected git command: $*" >&2
exit 64
EOF
chmod +x "$FAKE_BIN/go" "$FAKE_BIN/git"

set +e
(
	cd "$ROOT"
	GITHUB_OUTPUT="$GITHUB_OUTPUT_FILE" \
		GITHUB_TOKEN=test-token \
		PATH="$FAKE_BIN:$PATH" \
		bash "$STEP_SCRIPT"
) >"$OUTPUT" 2>&1
status=$?
set -e

[[ $status -eq 2 ]] ||
	fail "Resolve updates returned $status instead of failing with git diff status 2"
if grep -Fx 'changed=true' "$GITHUB_OUTPUT_FILE" >/dev/null; then
	fail "Resolve updates reported changes after git diff failed"
fi
