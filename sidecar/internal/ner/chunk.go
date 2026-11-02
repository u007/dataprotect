package ner

import (
	"fmt"
	"slices"
	"sort"

	"github.com/u007/dataprotect/recognizers"
)

// Chunking parameters from the design spec (§6, "Long text").
const (
	// ChunkTokens is the maximum number of tokens fed to one inference.
	ChunkTokens = 400
	// ChunkStride is the number of tokens two adjacent chunks share, so an
	// entity that straddles a boundary is seen whole by at least one chunk.
	ChunkStride = 64
	// MaxDocumentTokens is the hard cap on one document. A longer document is
	// refused: scanning only a prefix would leave the rest unredacted.
	MaxDocumentTokens = 8000
)

// tokenChunk is a half-open range of token indices, [Start, End).
type tokenChunk struct {
	Start int
	End   int
}

// planChunks returns the token ranges to run for a document of n tokens. A
// document longer than MaxDocumentTokens is an error: the caller must fail the
// request rather than scan a prefix and report the rest as clean.
func planChunks(n int) ([]tokenChunk, error) {
	if n <= 0 {
		return nil, nil
	}
	if n > MaxDocumentTokens {
		return nil, fmt.Errorf("onnx: document is %d tokens, over the %d-token cap", n, MaxDocumentTokens)
	}
	var chunks []tokenChunk
	step := ChunkTokens - ChunkStride
	for start := 0; start < n; start += step {
		end := min(start+ChunkTokens, n)
		chunks = append(chunks, tokenChunk{Start: start, End: end})
		if end == n {
			break
		}
	}
	return chunks, nil
}

// requiredInputs are the model inputs the backend cannot synthesize. Every
// other input it knows how to build; an input it does not know is an error, not
// a silent omission.
var requiredInputs = []string{"input_ids", "attention_mask"}

// buildFeeds turns one chunk of tokens into the model's named int64 feeds. The
// feed order follows inputNames, which is the session's declared order.
// token_type_ids is fed only when the model declares it, and zeros are correct
// for a single-segment input.
func buildFeeds(tokens []Token, inputNames []string) (map[string][]int64, error) {
	if len(inputNames) == 0 {
		return nil, fmt.Errorf("onnx: session declares no inputs")
	}
	feeds := make(map[string][]int64, len(inputNames))
	for _, name := range inputNames {
		switch name {
		case "input_ids":
			ids := make([]int64, len(tokens))
			for i, tok := range tokens {
				ids[i] = tok.ID
			}
			feeds[name] = ids
		case "attention_mask":
			mask := make([]int64, len(tokens))
			for i := range mask {
				mask[i] = 1
			}
			feeds[name] = mask
		case "token_type_ids":
			feeds[name] = make([]int64, len(tokens))
		default:
			return nil, fmt.Errorf("onnx: unsupported model input %q", name)
		}
	}
	for _, name := range requiredInputs {
		if _, ok := feeds[name]; !ok {
			return nil, fmt.Errorf("onnx: model is missing required input %q", name)
		}
	}
	return feeds, nil
}

// spanKey identifies an entity for cross-chunk deduplication. Identical
// (start, end, type) spans are produced twice when a chunk boundary falls
// inside an entity, because the stride makes both chunks see it.
type spanKey struct {
	start int
	end   int
	typ   recognizers.EntityType
}

// mergeChunkEntities deduplicates the per-chunk results and resolves the
// overlaps that remain between chunks. A span is dropped only when it overlaps
// a longer span that was kept, so a long chunk-edge artefact removes the spans
// it actually overlaps and nothing else. Decode has already broken ties by
// softmax probability inside a chunk, so the cross-chunk tie-break only needs to
// be deterministic: earlier start, then the smaller type name.
func mergeChunkEntities(perChunk [][]recognizers.Entity) []recognizers.Entity {
	seen := make(map[spanKey]bool)
	var all []recognizers.Entity
	for _, list := range perChunk {
		for _, e := range list {
			k := spanKey{start: e.Start, end: e.End, typ: e.Type}
			if seen[k] {
				continue
			}
			seen[k] = true
			all = append(all, e)
		}
	}
	if len(all) == 0 {
		return nil
	}

	// Longest first, so a span is only ever compared against longer or equal
	// spans already accepted. Ties: earlier start, then the smaller type name.
	sort.Slice(all, func(i, j int) bool {
		li := all[i].End - all[i].Start
		lj := all[j].End - all[j].Start
		if li != lj {
			return li > lj
		}
		if all[i].Start != all[j].Start {
			return all[i].Start < all[j].Start
		}
		return string(all[i].Type) < string(all[j].Type)
	})

	// accepted stays sorted by Start and pairwise disjoint. A candidate overlaps
	// an accepted span only when that span is the last one starting before the
	// candidate ends: disjoint spans have increasing ends, so no earlier one can
	// reach past the candidate's start. Overlap is checked pairwise, never
	// through a chain, so a span that overlaps only a rejected span is kept.
	var accepted []recognizers.Entity
	for _, e := range all {
		idx := sort.Search(len(accepted), func(i int) bool { return accepted[i].Start >= e.End })
		if idx > 0 && accepted[idx-1].End > e.Start {
			continue
		}
		accepted = slices.Insert(accepted, idx, e)
	}
	return accepted
}
