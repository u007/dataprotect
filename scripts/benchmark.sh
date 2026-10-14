#!/usr/bin/env bash
# Scores the golden dataset and prints the full report.
#
# Labels in testdata/golden are hand-written from the text, never copied from
# detector output — a gate that scores the detector against its own predictions
# cannot fail and proves nothing.
#
# Usage: scripts/benchmark.sh [dataset-dir]     (default: testdata/golden)
#
# If the directory contains a schema.json sidecar (the converted corpora do),
# its in_schema types are applied so out-of-schema detections are reported
# instead of counted as false positives. Without a sidecar the behaviour is
# exactly as it has always been for the hand-labelled seed set.
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
	"path/filepath"
	"sort"
	"strings"

	piiscrub "github.com/u007/dataprotect"
	"github.com/u007/dataprotect/golden"
)

func main() {
	dir := os.Args[1]
	cases, err := golden.Load(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchmark:", err)
		os.Exit(1)
	}

	// A converted corpus carries a schema declaring which entity types it
	// labels. Applying it stops a detection of a type the corpus never records
	// from being counted as a false positive — the FP-inflation trap TODO.md
	// records. A hand-labelled set has no sidecar and is scored exactly as it
	// always was: every detection is in scope, byte for byte unchanged.
	var opts []golden.Option
	if _, statErr := os.Stat(filepath.Join(dir, golden.SchemaFilename)); statErr == nil {
		sch, err := golden.LoadSchema(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "benchmark:", err)
			os.Exit(1)
		}
		opts = append(opts, golden.WithInSchema(sch.InSchema))
		types := make([]string, 0, len(sch.InSchema))
		for _, t := range sch.InSchema {
			types = append(types, string(t))
		}
		sort.Strings(types)
		fmt.Printf("schema:  %s\n", sch.Corpus)
		fmt.Printf("in_schema: [%s]\n", strings.Join(types, " "))
		fmt.Printf("sample:  %d cases of source_rows %d (limit %d)\n\n",
			sch.Cases, sch.SourceRows, sch.Limit)
	}

	s := piiscrub.NewScrubber()
	rep, err := golden.Score(context.Background(), dir,
		func(ctx context.Context, text string) ([]golden.Prediction, error) {
			return s.Analyze(ctx, text)
		}, opts...)
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