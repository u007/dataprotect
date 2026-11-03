#!/usr/bin/env bash
# Scores the golden dataset and prints the full report.
#
# Labels in testdata/golden are hand-written from the text, never copied from
# detector output — a gate that scores the detector against its own predictions
# cannot fail and proves nothing.
#
# Usage: scripts/benchmark.sh [dataset-dir] [case-limit]
#                                       (default: testdata/golden, no limit)
#
# Set PII_SCRUB_NLP_URL to score WITH the NLP sidecar's recognizer wired in,
# which adds PERSON, ORGANIZATION and LOCATION to the pipeline. The two
# numbers are then directly comparable: the same dataset, the same scorer, the
# only difference being the recognizer.
#
# Set PII_SCRUB_CARD_LANG to add a language's card-label phrases to the Luhn
# gate (comma-separated checksum.Language codes, e.g. sv). It is opt-in and
# combines with PII_SCRUB_NLP_URL: the default run is unchanged.
#
# The optional case limit scores the FIRST n cases. It exists because the
# sidecar costs about 20 ms per text, so a full 2000-case corpus takes about
# 40 seconds. A limited run prints the number of cases it actually scored, so
# a sample can never be read as a full-corpus figure.
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
# The optional case limit, passed through to the program as a second argument.
# An empty value means "no limit", and the program treats a missing argument
# the same way.
LIMIT_ARG="${2:-}"

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
	"strconv"
	"strings"

	piiscrub "github.com/u007/dataprotect"
	"github.com/u007/dataprotect/golden"
	"github.com/u007/dataprotect/recognizers/checksum"
	"github.com/u007/dataprotect/recognizers/nlp"
)

func main() {
	dir := os.Args[1]
	if _, err := golden.Load(dir); err != nil {
		fmt.Fprintln(os.Stderr, "benchmark:", err)
		os.Exit(1)
	}
	limit := 0
	if len(os.Args) > 2 {
		n, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "benchmark: case limit must be a number:", err)
			os.Exit(1)
		}
		limit = n
	}

	// A converted corpus carries a schema declaring which entity types it
	// labels. Applying it stops a detection of a type the corpus never records
	// from being counted as a false positive — the FP-inflation trap TODO.md
	// records. A hand-labelled set has no sidecar and is scored exactly as it
	// always was: every detection is in scope, byte for byte unchanged.
	var opts []golden.Option
	// corpusCases is the corpus's full case count, from the schema sidecar. It
	// is 0 for a hand-labelled set, which has no schema.
	corpusCases := 0
	if _, statErr := os.Stat(filepath.Join(dir, golden.SchemaFilename)); statErr == nil {
		sch, err := golden.LoadSchema(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "benchmark:", err)
			os.Exit(1)
		}
		opts = append(opts, golden.WithInSchema(sch.InSchema))
		corpusCases = sch.Cases
		types := make([]string, 0, len(sch.InSchema))
		for _, t := range sch.InSchema {
			types = append(types, string(t))
		}
		sort.Strings(types)
		fmt.Printf("schema:  %s\n", sch.Corpus)
		fmt.Printf("in_schema: [%s]\n", strings.Join(types, " "))
		fmt.Printf("sample:  %d cases of source_rows %d (limit %d)\n",
			sch.Cases, sch.SourceRows, sch.Limit)
	}

	// The NLP recognizer is opt-in, so the benchmark is opt-in too. A URL
	// without a sidecar running scores exactly as it always has.
	// Card-label languages are opt-in as well: PII_SCRUB_CARD_LANG=sv scores
	// the split with the Swedish phrases added to the English default.
	var scrubOpts []piiscrub.ScrubberOption
	if raw := os.Getenv("PII_SCRUB_CARD_LANG"); raw != "" {
		var langs []checksum.Language
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				langs = append(langs, checksum.Language(p))
			}
		}
		scrubOpts = append(scrubOpts, piiscrub.WithCardLanguages(langs...))
		fmt.Printf("card-languages: %s\n\n", raw)
	}
	if url := os.Getenv("PII_SCRUB_NLP_URL"); url != "" {
		scrubOpts = append(scrubOpts, piiscrub.WithRecognizer(nlp.New(url)))
		fmt.Printf("nlp:     %s\n\n", url)
	}
	s := piiscrub.NewScrubber(scrubOpts...)
	if limit > 0 {
		opts = append(opts, golden.WithCaseLimit(limit))
	}
	rep, err := golden.Score(context.Background(), dir,
		func(ctx context.Context, text string) ([]golden.Prediction, error) {
			return s.Analyze(ctx, text)
		}, opts...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchmark:", err)
		os.Exit(1)
	}

	// The case count must be the number ACTUALLY scored, not the number
	// loaded, or a limited run reads as a full-corpus figure.
	// A limited run must say so, next to the number: the schema records the
	// FULL corpus size, and a reader who sees 2000 there and 100 here must be
	// told which one the score covers.
	if limit > 0 && corpusCases > 0 && rep.Cases < corpusCases {
		fmt.Printf("limited: %d of %d cases scored\n", rep.Cases, corpusCases)
	} else {
		fmt.Printf("cases: %d\n", rep.Cases)
	}
	fmt.Println()
	fmt.Print(rep.Summary())

	if len(rep.Errors) > 0 {
		os.Exit(1)
	}
}
GO

if [ -n "$LIMIT_ARG" ]; then
	go run "$TMP/main.go" "$DIR" "$LIMIT_ARG"
else
	go run "$TMP/main.go" "$DIR"
fi