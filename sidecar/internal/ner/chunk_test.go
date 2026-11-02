package ner

import (
	"reflect"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

func TestPlanChunks(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want []tokenChunk
	}{
		{name: "empty", n: 0, want: nil},
		{name: "single token", n: 1, want: []tokenChunk{{0, 1}}},
		{name: "exactly one chunk", n: ChunkTokens, want: []tokenChunk{{0, ChunkTokens}}},
		{name: "one token over", n: ChunkTokens + 1, want: []tokenChunk{{0, ChunkTokens}, {ChunkTokens - ChunkStride, ChunkTokens + 1}}},
		{name: "two full chunks", n: 2 * (ChunkTokens - ChunkStride), want: []tokenChunk{{0, ChunkTokens}, {ChunkTokens - ChunkStride, 2 * (ChunkTokens - ChunkStride)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := planChunks(tt.n)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("planChunks(%d) = %v, want %v", tt.n, got, tt.want)
			}
		})
	}
}

func TestPlanChunksCoversEveryToken(t *testing.T) {
	for _, n := range []int{1, 337, 400, 401, 1000, MaxDocumentTokens} {
		chunks, err := planChunks(n)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		seen := make([]bool, n)
		for _, c := range chunks {
			for i := c.Start; i < c.End; i++ {
				seen[i] = true
			}
		}
		for i, ok := range seen {
			if !ok {
				t.Fatalf("n=%d: token %d is in no chunk", n, i)
			}
		}
		if last := chunks[len(chunks)-1]; last.End != n {
			t.Fatalf("n=%d: last chunk ends at %d", n, last.End)
		}
	}
}

func TestPlanChunksRefusesOverCap(t *testing.T) {
	chunks, err := planChunks(MaxDocumentTokens + 1)
	if err == nil {
		t.Fatalf("expected an error for a document over the cap, got %d chunks", len(chunks))
	}
	if chunks != nil {
		t.Fatalf("chunks returned alongside an error: %v", chunks)
	}
}

func TestBuildFeeds(t *testing.T) {
	tokens := []Token{{ID: 5}, {ID: 6}, {ID: 7}}

	t.Run("required inputs only", func(t *testing.T) {
		feeds, err := buildFeeds(tokens, []string{"input_ids", "attention_mask"})
		if err != nil {
			t.Fatal(err)
		}
		wantIDs := []int64{5, 6, 7}
		wantMask := []int64{1, 1, 1}
		if !reflect.DeepEqual(feeds["input_ids"], wantIDs) {
			t.Fatalf("input_ids = %v, want %v", feeds["input_ids"], wantIDs)
		}
		if !reflect.DeepEqual(feeds["attention_mask"], wantMask) {
			t.Fatalf("attention_mask = %v, want %v", feeds["attention_mask"], wantMask)
		}
		if _, ok := feeds["token_type_ids"]; ok {
			t.Fatal("token_type_ids fed although the model does not declare it")
		}
	})

	t.Run("token_type_ids when declared", func(t *testing.T) {
		feeds, err := buildFeeds(tokens, []string{"input_ids", "token_type_ids", "attention_mask"})
		if err != nil {
			t.Fatal(err)
		}
		if got := feeds["token_type_ids"]; !reflect.DeepEqual(got, []int64{0, 0, 0}) {
			t.Fatalf("token_type_ids = %v, want zeros", got)
		}
	})

	t.Run("missing required input", func(t *testing.T) {
		if _, err := buildFeeds(tokens, []string{"input_ids"}); err == nil {
			t.Fatal("expected error for a model with no attention_mask input")
		}
	})

	t.Run("unknown input", func(t *testing.T) {
		if _, err := buildFeeds(tokens, []string{"input_ids", "attention_mask", "position_ids"}); err == nil {
			t.Fatal("expected error for an input the backend cannot build")
		}
	})

	t.Run("no inputs", func(t *testing.T) {
		if _, err := buildFeeds(tokens, nil); err == nil {
			t.Fatal("expected error for a session with no inputs")
		}
	})
}

func TestMergeChunkEntitiesDeduplicates(t *testing.T) {
	person := recognizers.Entity{Type: recognizers.EntityPerson, Start: 10, End: 16, Text: "Berlin"}
	other := recognizers.Entity{Type: recognizers.EntityPerson, Start: 10, End: 16, Text: "Berlin"}

	got := mergeChunkEntities([][]recognizers.Entity{{person}, {other}})
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1: %v", len(got), got)
	}
}

func TestMergeChunkEntitiesKeepsDistinctTypesAtSameOffsets(t *testing.T) {
	loc := recognizers.Entity{Type: recognizers.EntityLocation, Start: 10, End: 16, Text: "Berlin"}
	person := recognizers.Entity{Type: recognizers.EntityPerson, Start: 10, End: 16, Text: "Berlin"}

	// Same offsets but different types still overlap, so the longest-wins
	// rule keeps one; the tie-break is the smaller type name.
	got := mergeChunkEntities([][]recognizers.Entity{{loc}, {person}})
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1: %v", len(got), got)
	}
	if got[0].Type != recognizers.EntityLocation {
		t.Fatalf("kept %s, want LOCATION (smaller type name)", got[0].Type)
	}
}

func TestMergeChunkEntitiesLongestWinsAcrossChunks(t *testing.T) {
	short := recognizers.Entity{Type: recognizers.EntityPerson, Start: 10, End: 16, Text: "Berlin"}
	long := recognizers.Entity{Type: recognizers.EntityPerson, Start: 10, End: 23, Text: "Berlin Germany"}

	got := mergeChunkEntities([][]recognizers.Entity{{short}, {long}})
	if len(got) != 1 || got[0].End != 23 {
		t.Fatalf("got %v, want the longer span", got)
	}
}

func TestMergeChunkEntitiesKeepsDisjoint(t *testing.T) {
	a := recognizers.Entity{Type: recognizers.EntityPerson, Start: 0, End: 5}
	b := recognizers.Entity{Type: recognizers.EntityLocation, Start: 6, End: 10}

	got := mergeChunkEntities([][]recognizers.Entity{{a}, {b}})
	if len(got) != 2 {
		t.Fatalf("got %d entities, want 2: %v", len(got), got)
	}
	if got[0].Start != 0 || got[1].Start != 6 {
		t.Fatalf("entities are not sorted by start: %v", got)
	}
}

func TestMergeChunkEntitiesLongEdgeSpanKeepsCorrectShorterSpans(t *testing.T) {
	// The long span starting at 5 is a chunk-edge artefact. The span [2,6) overlaps
	// it and is rejected. The correct span [0,3) overlaps only the rejected span,
	// so it must survive; a transitive group would drop it. [31,35) is disjoint.
	artefact := recognizers.Entity{Type: recognizers.EntityOrganization, Start: 5, End: 30, Text: "artefact"}
	middle := recognizers.Entity{Type: recognizers.EntityLocation, Start: 2, End: 6, Text: "middle"}
	correct := recognizers.Entity{Type: recognizers.EntityPerson, Start: 0, End: 3, Text: "Ann"}
	disjoint := recognizers.Entity{Type: recognizers.EntityPerson, Start: 31, End: 35, Text: "Bob"}

	got := mergeChunkEntities([][]recognizers.Entity{{artefact, disjoint}, {middle, correct}})
	want := []recognizers.Entity{correct, artefact, disjoint}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
