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
	"strings"
	"unicode/utf8"

	piiscrub "github.com/u007/dataprotect"
	"github.com/u007/dataprotect/golden"
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
		checked, collisions := 0, 0
		for _, c := range cases {
			out, err := scrubber.Anonymize(ctx, c.Text)
			if err != nil {
				fail(fmt.Errorf("%s: case %q: %w", dir, c.ID, err))
			}
			if !utf8.ValidString(out) {
				fail(fmt.Errorf("%s: case %q: output is not valid UTF-8", dir, c.ID))
			}
			pseudo, pseudonyms, err := scrubber.Pseudonymize(ctx, c.Text)
			if err != nil {
				if strings.Contains(err.Error(), "placeholder-shaped") {
					collisions++
					continue
				}
				fail(fmt.Errorf("%s: case %q: pseudonymize: %w", dir, c.ID, err))
			}
			if !utf8.ValidString(pseudo) {
				fail(fmt.Errorf("%s: case %q: pseudonymized output is not valid UTF-8", dir, c.ID))
			}
			if !golden.PseudonymRoundTrip(c.Text, pseudo, pseudonyms) {
				fail(fmt.Errorf("%s: case %q: restoring the placeholders does not reproduce the input", dir, c.ID))
			}
			checked++
		}
		fmt.Printf("%s: %d cases anonymized, no errors, all output valid UTF-8; pseudonyms round-trip on %d, %d collisions set aside\n", dir, len(cases), checked, collisions)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "corpusanon:", err)
	os.Exit(1)
}
