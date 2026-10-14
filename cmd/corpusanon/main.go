// Command corpusanon runs Scrubber.Anonymize over every case in one or more
// converted corpus directories and fails on the first error.
//
// It checks the anonymizer contract at corpus scale, not detection quality: each
// call must return no error, and the output must be valid UTF-8. Detection
// quality is scored by `make corpus`. This is a developer tool, not CI, because
// the corpora are gitignored.
//
// Usage:
//
//	corpusanon <converted-dir>...
package main

import (
	"context"
	"fmt"
	"os"
	"unicode/utf8"

	piiscrub "dataprotect"
	"dataprotect/golden"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: corpusanon <converted-dir>...")
		os.Exit(2)
	}
	scrubber := piiscrub.NewScrubber()
	ctx := context.Background()
	for _, dir := range os.Args[1:] {
		cases, err := golden.Load(dir)
		if err != nil {
			fail(err)
		}
		for _, c := range cases {
			out, err := scrubber.Anonymize(ctx, c.Text)
			if err != nil {
				fail(fmt.Errorf("%s: case %q: %w", dir, c.ID, err))
			}
			if !utf8.ValidString(out) {
				fail(fmt.Errorf("%s: case %q: output is not valid UTF-8", dir, c.ID))
			}
		}
		fmt.Printf("%s: %d cases anonymized, no errors, all output valid UTF-8\n", dir, len(cases))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "corpusanon:", err)
	os.Exit(1)
}
