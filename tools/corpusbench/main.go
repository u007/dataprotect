// Command corpusbench converts benchmark corpora into golden datasets.
//
// It is a developer tool, not shipped library API: it lives in its own module
// (tools/corpusbench) so the parquet reader never reaches the root module's
// dependency list, which the project keeps stdlib-only.
//
// Usage:
//
//	corpusbench convert <corpus> <data-root> <limit> [lang]
//
// Positional arguments only, deliberately. There is no --dry-run and no
// --verbose: a dry-run flag that disagreed with the real run would be a second
// code path to keep honest, and verbosity that changed behaviour would make two
// runs of the same conversion incomparable.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/u007/dataprotect/golden"
	"github.com/u007/dataprotect/tools/corpusbench/convert"
)

const usage = `usage: corpusbench convert <corpus> <data-root> <limit> [lang]

  corpus    nemotron-pii | gretel-pii-finance
  data-root directory fetch-dataset.sh populated (e.g. testdata/external)
  limit     records to convert, in file order; 0 means all of them
  lang      optional gretel split: English | France | German | Italian |
            Spanish | Swedish | Dutch. Omit to convert the combined test split.

Output is written to <data-root>/<corpus>/converted/, or to
<data-root>/<corpus>-<lang>/converted/ when lang is given.`

func main() {
	if (len(os.Args) != 5 && len(os.Args) != 6) || os.Args[1] != "convert" {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	corpus := os.Args[2]
	dataRoot := os.Args[3]
	limit, err := strconv.Atoi(os.Args[4])
	if err != nil || limit < 0 {
		fmt.Fprintf(os.Stderr, "corpusbench: limit must be a non-negative integer, got %q\n", os.Args[4])
		os.Exit(2)
	}
	lang := ""
	if len(os.Args) == 6 {
		lang, err = normalizeLang(os.Args[5])
		if err != nil {
			fmt.Fprintf(os.Stderr, "corpusbench: %v\n", err)
			os.Exit(2)
		}
	}

	cases, schema, err := run(corpus, dataRoot, limit, lang)
	if err != nil {
		fmt.Fprintf(os.Stderr, "corpusbench: %v\n", err)
		os.Exit(1)
	}

	outDir := filepath.Join(dataRoot, outputName(corpus, lang), "converted")
	if err := convert.Write(outDir, cases, schema); err != nil {
		fmt.Fprintf(os.Stderr, "corpusbench: %v\n", err)
		os.Exit(1)
	}

	// The counts are printed because a conversion that silently produced far
	// fewer records than expected is otherwise indistinguishable from a good
	// one until somebody reads the report it fed.
	fmt.Printf("corpusbench: %s -> %s\n", corpus, outDir)
	fmt.Printf("  cases %d of source_rows %d (limit %d)\n",
		schema.Cases, schema.SourceRows, schema.Limit)
	fmt.Printf("  negatives %d, overlapping_pairs %d, unmapped_labels %d\n",
		schema.Negatives, schema.OverlappingPairs, len(schema.UnmappedLabels))
	fmt.Printf("  revision %s\n", schema.SourceRevision)
}

// normalizeLang trims the raw lang argument. An argument that is empty after
// trimming is an error: passing it would otherwise select the combined split
// while the caller asked for a language.
func normalizeLang(raw string) (string, error) {
	lang := strings.TrimSpace(raw)
	if lang == "" {
		return "", fmt.Errorf("lang must not be empty; omit the argument to convert the combined split")
	}
	return lang, nil
}

// outputName is the directory name under data-root for a conversion. lang is
// expected to be normalized already, so the same value reaches both this name
// and run.
func outputName(corpus, lang string) string {
	if lang == "" {
		return corpus
	}
	return corpus + "-" + strings.ToLower(lang)
}

// run dispatches on the corpus name. An unknown name is a hard error rather
// than a silent default: converting the wrong corpus would produce a report
// labelled as the requested one.
func run(corpus, dataRoot string, limit int, lang string) ([]golden.Case, golden.Schema, error) {
	switch corpus {
	case "nemotron-pii":
		if lang != "" {
			return nil, golden.Schema{}, fmt.Errorf("corpusbench: lang is only supported for gretel-pii-finance, not %q", corpus)
		}
		cases, schema, err := convert.Nemotron(dataRoot, limit)
		return cases, schema, err
	case "gretel-pii-finance":
		if lang != "" {
			return convert.GretelLang(dataRoot, limit, lang)
		}
		cases, schema, err := convert.Gretel(dataRoot, limit)
		return cases, schema, err
	default:
		return nil, golden.Schema{}, fmt.Errorf("unknown corpus %q; want nemotron-pii or gretel-pii-finance", corpus)
	}
}
