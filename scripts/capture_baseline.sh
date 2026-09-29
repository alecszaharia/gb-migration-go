#!/usr/bin/env bash
# Capture the golden baseline of the pre-change sequential global_blocks tool.
#
# Builds commit BASE_COMMIT (default 7c5eea1) in a temporary git worktree,
# then runs TestCaptureGoldenBaseline, which seeds the deterministic golden
# fixture into throwaway MySQL schemas, runs the old binary against them and
# writes internal/cli/global_blocks/testdata/golden/{normal,failed}.json.
#
# Usage:
#   GB_MIGRATION_TEST_DSN='root:nopassword@tcp(127.0.0.1:3306)/' scripts/capture_baseline.sh
#
# Environment:
#   GB_MIGRATION_TEST_DSN  required; server DSN (the harness creates and drops
#                          gbtest_* schemas, existing databases are untouched)
#   BASE_COMMIT            commit of the pre-change tool (default 7c5eea1)
#   GIT                    git binary to use (default: git)
set -euo pipefail

: "${GB_MIGRATION_TEST_DSN:?set GB_MIGRATION_TEST_DSN, e.g. root:nopassword@tcp(127.0.0.1:3306)/}"
BASE_COMMIT="${BASE_COMMIT:-7c5eea1}"
GIT="${GIT:-git}"

ROOT="$("$GIT" rev-parse --show-toplevel)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/gb-baseline.XXXXXX")"
WORKTREE="$TMP/src"
BIN="$TMP/gb-baseline"

cleanup() {
	"$GIT" -C "$ROOT" worktree remove --force "$WORKTREE" >/dev/null 2>&1 || true
	"$GIT" -C "$ROOT" worktree prune >/dev/null 2>&1 || true
	rm -rf "$TMP"
}
trap cleanup EXIT

echo "==> checking out $BASE_COMMIT into $WORKTREE"
"$GIT" -C "$ROOT" worktree add --detach "$WORKTREE" "$BASE_COMMIT" >/dev/null

echo "==> building baseline tool"
(cd "$WORKTREE" && go build -o "$BIN" .)

echo "==> capturing golden files"
cd "$ROOT"
GB_CAPTURE_GOLDEN_BIN="$BIN" go test -count=1 -run '^TestCaptureGoldenBaseline$' -v ./internal/cli/global_blocks/

echo "==> golden files written:"
ls -l internal/cli/global_blocks/testdata/golden/
