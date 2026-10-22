package ner

import (
	"fmt"
	"testing"

	"github.com/jdkato/prose/v2"
)

func TestTrace(t *testing.T) {
	for _, txt := range []string{
		"Brian, born on 1963-08-08, resides at 146 County Rd 86 in Knox County, USA.",
		"Example Case Study: Betty Norris and 462 Mills Mountain",
	} {
		doc, _ := prose.NewDocument(txt)
		toks := doc.Tokens()
		fmt.Printf(">> %q\n", txt)
		for i := 0; i < len(toks); {
			run, next := collectRun(toks, i)
			i = next
			if len(run) == 0 {
				continue
			}
			words := []string{}
			for _, x := range run {
				words = append(words, x.Text)
			}
			typ, adj, ok := classify(run)
			fmt.Printf("   run=%v -> type=%q ok=%v adj=%v\n", words, typ, ok, len(adj))
		}
	}
}
