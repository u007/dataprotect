// Command corpusbench converts benchmark corpora into golden datasets.
//
// It is a developer tool, not shipped library API: it lives in its own module
// (tools/corpusbench) so the parquet reader never reaches the root module's
// dependency list, which the project keeps stdlib-only.
//
// Usage:
//
//	corpusbench convert <corpus> <data-root> <limit>
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

	"github.com/u007/dataprotect/golden"
	"github.com/u007/dataprotect/tools/corpusbench/convert"
)

const usage = `usage: corpusbench convert <corpus> <data-root> <limit>

  corpus    nemotron-pii | gretel-pii-finance
  data-root directory fetch-dataset.sh populated (e.g. testdata/external)
  limit     records to convert, in file order; 0 means all of them

Output is written to <data-root>/<corpus>/converted/`

func main() {
	if len(os.Args) != 5 || os.Args[1] != "convert" {
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

	cases, schema, err := run(corpus, dataRoot, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "corpusbench: %v\n", err)
		os.Exit(1)
	}

	outDir := filepath.Join(dataRoot, corpus, "converted")
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

// run dispatches on the corpus name. An unknown name is a hard error rather
// than a silent default: converting the wrong corpus would produce a report
// labelled as the requested one.
func run(corpus, dataRoot string, limit int) ([]golden.Case, golden.Schema, error) {
	switch corpus {
	case "nemotron-pii":
		cases, schema, err := convert.Nemotron(dataRoot, limit)
		return cases, schema, err
	case "gretel-pii-finance":
		cases, schema, err := convert.Gretel(dataRoot, limit)
		return cases, schema, err
	default:
		return nil, golden.Schema{}, fmt.Errorf("unknown corpus %q; want nemotron-pii or gretel-pii-finance", corpus)
	}
}
