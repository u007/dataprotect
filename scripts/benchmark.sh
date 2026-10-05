#!/usr/bin/env bash
# Scores the golden dataset and prints the full report.
#
# Labels in testdata/golden are hand-written from the text, never copied from
# detector output — a gate that scores the detector against its own predictions
# cannot fail and proves nothing.
#
# Usage: scripts/benchmark.sh [dataset-dir]     (default: testdata/golden)
#
# Exits non-zero when the dataset cannot be loaded at all, so a missing or
# malformed corpus is visibly an error rather than an empty perfect score.
set -euo pipefail

cd "$(dirname "$0")/.."

DIR="${1:-testdata/golden}"

if [ ! -d "$DIR" ]; then
  echo "benchmark: dataset directory not found: $DIR" >&2
  exit 1
fi

echo "benchmark: scoring $DIR"
echo

# `go run` with a throwaway file outside the module tree: the benchmark is a
# developer tool, not shipped API surface.
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat >"$TMP/main.go" <<'GO'
package main

import (
	"context"
	"fmt"
	"os"

	piiscrub "dataprotect"
	"dataprotect/golden"
)

func main() {
	dir := os.Args[1]
	cases, err := golden.Load(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchmark:", err)
		os.Exit(1)
	}

	s := piiscrub.NewScrubber()
	rep, err := golden.Score(context.Background(), dir,
		func(ctx context.Context, text string) ([]golden.Prediction, error) {
			return s.Analyze(ctx, text)
		})
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchmark:", err)
		os.Exit(1)
	}

	fmt.Printf("cases: %d\n\n", len(cases))
	fmt.Print(rep.Summary())

	if len(rep.Errors) > 0 {
		os.Exit(1)
	}
}
GO

go run "$TMP/main.go" "$DIR"