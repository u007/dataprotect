# Corpus Converter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (default) or superpowers:subagent-driven-development (long plans of independent tasks) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the two fetched corpora into scoreable golden datasets and add a
scoring mode that measures P/R without inflating false positives.

**Architecture:** A new isolated Go module (`tools/corpusbench`) converts
Parquet corpora into ordinary `golden.Case` JSON plus a `schema.json` sidecar,
writing only into gitignored `testdata/external/`. The `golden` package gains
three additive pieces — sidecar loading, an in-schema scoring filter, and
optional byte offsets on labels — so the corpus measurement and the CI gate keep
one shared scorer.

**Tech Stack:** Go 1.24 (stdlib-only in the root module); parquet-go pinned in
the tool module only; Bash Makefile targets.

**Spec:** `docs/superpowers/specs/2026-10-08-corpus-converter-design.md` (approved
2026-10-08). The plan argues from the spec; executors read both.

**Detail:** decisions, not code — the default; the implementer writes the
implementation.

## Global Constraints

Every task inherits these; they are copied from the spec, not invented here.

- Root module stays **stdlib-only** (`AGENTS.md` §Code Conventions). parquet-go
  belongs only in `tools/corpusbench/go.mod`, pinned to an exact version.
- Tool module: `module dataprotect/tools/corpusbench` with
  `replace dataprotect => ../..`. No library package imports the tool. No root
  `go.mod` change.
- **No corpus-derived bytes committed.** All conversion output goes under
  `testdata/external/`, which `.gitignore` already covers.
- **Fail fast, never skip.** A record the converter cannot convert aborts the run
  with an error carrying corpus, source file, row index, and span index. No
  skip-and-continue, no partial output.
- **No mode flags.** CLI is positional: `convert <corpus> <data-root> <limit>`.
  No `dryRun`, no `verbose`, no boolean toggles. The limit is the Makefile
  variable `LIMIT ?= 2000`.
- **Deterministic output** (`AGENTS.md`): case IDs derive from source row index;
  case files are a sorted, fixed-width sequence; labels ordered by first
  in-schema offset; `in_schema` and `unmapped_labels` arrays sorted.
- Reserved sidecar filename is exactly `schema.json`.
- `schema.json` records `source_rows`, `limit`, `cases`, `source_revision`, and
  `source_sha256` so a sampled report can never be mistaken for a full-corpus one.
- **No existing test is modified or removed.**
- Exported types and functions get doc comments; tests live beside source as
  `*_test.go`; fixtures are synthetic only, never real PII (`AGENTS.md`).
- `DATAPROTECT_DATA_DIR` gets documented in `.env.example` (optional, defaults to
  `testdata/external`, shared with `scripts/fetch-dataset.sh`).

## Review Focus

Five inputs most likely to bite a person using this software, each with the task
that owns its test:

1. **Character vs byte offsets.** Python char offsets sliced as bytes give
   *wrong but plausible* label text on any non-ASCII record (measured: 3614/6712
   gretel spans differ, 132 land mid-rune). Expect: every converted span
   selects exactly the corpus's intended text. → Task 4.
2. **Out-of-schema detection counted as FP.** A corpus labelling only PHONE
   makes a correct EMAIL detection a false positive. Expect: excluded from
   FP/FN, reported in `Report.OutOfSchema`. → Task 2.
3. **Sidecar collides with the loader.** `Load` globs `*.json`, so `schema.json`
   beside cases makes the whole dataset fail to parse. Expect: cases load
   normally, sidecar readable separately. → Task 1.
4. **Mid-rune span boundary.** A span whose char offsets land mid-rune emits
   invalid UTF-8. Expect: conversion aborts naming the row, never emits. → Task 4.
5. **Repeated value in one record.** `resolve()` rejects a label whose value
   occurs twice, discarding the case (measured: 13.9%/14.9% of positives). Expect:
   offset labels resolve without uniqueness checking. → Task 3.

---

### Task 1: Schema sidecar in `golden`

**Risk:** standard — additive file-loading behaviour, no scoring change.

**Files:**
- Create: `golden/schema.go`
- Modify: `golden/golden.go` (`Load`, ~:342-400 — skip the reserved name)
- Test: `golden/schema_test.go`

**Interfaces:**
- Consumes: `recognizers.EntityType`, `recognizers.EntityTypeFromString`
  (validation), the `DisallowUnknownFields` decoding pattern `Load` already uses.
- Produces:
  - `type Schema struct` — JSON fields `corpus`, `source`, `source_sha256`,
    `source_revision`, `source_rows`, `limit`, `cases`, `offset_unit`,
    `in_schema []recognizers.EntityType`, `unmapped_labels []string`,
    `overlapping_pairs`, `negatives`, `generated_by`.
  - `func LoadSchema(dir string) (Schema, error)`
  - constant for the reserved filename `schema.json`, referenced by `Load`.

- [ ] **Step 1: Write the failing tests**

```go
func TestLoadSkipsSchemaSidecar(t *testing.T) {
	dir := t.TempDir()
	// write cases-000.json = [ one Case ], schema.json = a Schema object
	cases, err := Load(dir)
	// want: err == nil, len(cases) == 1, ID preserved
}

func TestLoadSchemaRejectsUnknownField(t *testing.T) {
	// schema.json containing {"corpus":"x","nonsense":1}
	// want: LoadSchema returns error naming "nonsense"
}

func TestLoadSchemaRejectsCaseArray(t *testing.T) {
	// schema.json containing a JSON array (a misnamed case file)
	// want: LoadSchema returns an error, not a zero Schema
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./golden/ -run 'TestLoadSkipsSchemaSidecar|TestLoadSchema' -v`
Expected: FAIL (`LoadSchema` undefined; sidecar not yet skipped).

- [ ] **Step 3: Implement `LoadSchema` and the reserved-name skip**

Define `Schema` with doc comments and `json` tags matching the field list above;
`LoadSchema` decodes with `DisallowUnknownFields` and validates each
`in_schema` entry through `recognizers.EntityTypeFromString`. `Load` ignores
exactly the reserved filename. Document the reservation in `Load`'s doc comment.

- [ ] **Step 4: Run the tests and the whole package**

Run: `go test ./golden/ -v && go test ./...`
Expected: PASS, including every pre-existing test unchanged.

---

### Task 2: In-schema scoring filter (the FP trap)

**Risk:** high — changes a public scoring contract; the failure mode (inflated
precision/false-positive counts) is silent and would make a published number wrong.

**Files:**
- Create: `golden/filter.go`
- Modify: `golden/golden.go` (`Score` ~:227 gains a variadic option; `Report`
  ~:159 gains the out-of-schema bucket; `Summary` ~:403 prints it)
- Test: `golden/filter_test.go`

**Interfaces:**
- Consumes: `Schema` + `LoadSchema` (Task 1), `scorePair`, `filterType`.
- Produces:
  - `type Option func(*scoreConfig)`
  - `func WithInSchema(types []recognizers.EntityType) Option`
  - `func Score(ctx context.Context, dir string, predict func(context.Context,
    string) ([]Prediction, error), opts ...Option) (*Report, error)` — variadic,
    so existing call sites (`golden/accuracy_test.go:85`, `scripts/benchmark.sh`)
    compile unchanged.
  - `Report.OutOfSchema map[string]int` (counts by type; nil/empty when no filter).

- [ ] **Step 1: Write the failing test — the trap TODO.md describes**

```go
func TestOutOfSchemaDetectionIsNotCountedAsFP(t *testing.T) {
	dir := t.TempDir() // cases-000.json: Text "Call 555-123-4567 or a@b.com today."
	//   entities: one PHONE label, value "555-123-4567" (byte 5..17)
	//   schema.json: in_schema = ["PHONE"]
	predict := func(context.Context, string) ([]Prediction, error) {
		return []Prediction{
			{Type: "PHONE", Start: 5, End: 17, Text: "555-123-4567", Confidence: 1},
			// Out-of-schema: the corpus has no EMAIL label at all.
			{Type: "EMAIL", Start: 21, End: 28, Text: "a@b.com", Confidence: 1},
			// IN-SCHEMA with no gold — a real false positive the filter must
			// NOT swallow. Omitting this lets a filter that excludes every
			// unmatched prediction still score FP == 0 and pass.
			{Type: "PHONE", Start: 33, End: 45, Text: "555-999-0000", Confidence: 1},
		}, nil
	}
	rep, err := Score(context.Background(), dir, predict,
		WithInSchema([]recognizers.EntityType{"PHONE"}))
	// want: err == nil
	// want: rep.Exact.TP == 1  (the first PHONE)
	// want: rep.Exact.FP == 1  <-- the unmatched IN-SCHEMA PHONE stays a real FP
	// want: rep.Exact.FP == 0 for EMAIL alone is what the filter buys:
	//       without it FP == 2 (EMAIL + the second PHONE)
	// want: rep.OutOfSchema["EMAIL"] == 1
	// want: rep.OutOfSchema has no "PHONE" key
}

func TestScoreRejectsGoldOutsideSchema(t *testing.T) {
	// Gold whose type is absent from in_schema can never match, because every
	// prediction of that type is filtered out first. Scoring it would silently
	// count FNs for entities the detector was never allowed to report — a
	// quiet wrong number. Fail instead.
	dir := t.TempDir() // cases-000.json labels EMAIL; schema.json in_schema = ["PHONE"]
	_, err := Score(context.Background(), dir, func(context.Context, string) ([]Prediction, error) {
		return nil, nil
	}, WithInSchema([]recognizers.EntityType{recognizers.EntityPhone}))
	// want: err != nil, message names both the offending type and the schema
	//       (strings.Contains(err.Error(), "EMAIL"))
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./golden/ -run TestOutOfSchemaDetectionIsNotCountedAsFP -v`
Expected: FAIL — `WithInSchema` undefined (feature absent).

- [ ] **Step 3: Prove the trap exists without the filter**

Add a second assertion in the same file: calling `Score` with **no** option over
the same dataset yields `rep.Exact.FP == 1` and an empty `OutOfSchema`.
Run it — it should PASS (documents today's behaviour and pins that the default
is byte-identical for the seed set).

- [ ] **Step 4: Implement `Option` / `WithInSchema` and the filter**

Config holds the in-schema set (nil = no filter). `Score` drops out-of-schema
predictions **before** exact scoring, overlap scoring, and the `ByType` loop,
counting them by type into `Report.OutOfSchema`. `Summary` prints an
out-of-schema line only when the bucket is non-empty.

- [ ] **Step 5: Run tests**

Run: `go test ./golden/ -v && go test ./...`
Expected: PASS; both the new test and the no-filter default test pass.

---

### Task 3: Offset labels on `golden.Case`

**Risk:** high — public data contract; backwards compatibility with the
committed 18-case seed set must be exact, and boundary validation is where
invalid UTF-8 would slip through.

**Files:**
- Modify: `golden/golden.go` (`Label` ~:59, `resolve` ~:71-107)
- Test: `golden/offset_test.go`

**Interfaces:**
- Consumes: nothing new — operates inside `resolve`.
- Produces:
  - `Label.Start *int` / `Label.End *int`, JSON `start`/`end`, `omitempty`.
    Pointers, so a span at byte 0 is not confused with "no offsets".
  - `resolve()` behaviour: both set → use them directly (validate bounds,
    `Start < End`, both on rune boundaries, and `Text[Start:End] == Value` when
    `Value` is non-empty). Both nil → the existing substring path, unchanged.
    Exactly one set → error.

- [ ] **Step 1: Write the failing tests**

```go
func TestResolveUsesExplicitOffsetsForRepeatedValues(t *testing.T) {
	c := Case{
		ID:   "dup",
		Text: "Card 4111 1111 1111 1111 and 4111 1111 1111 1111 again",
		Entities: []Label{
			{Type: "CREDIT_CARD", Value: "4111 1111 1111 1111", Start: sp(5), End: ep(24)},
			{Type: "CREDIT_CARD", Value: "4111 1111 1111 1111", Start: sp(29), End: ep(48)},
		},
	}
	gold, err := c.resolve()
	// want: err == nil, two distinct spans 5..24 and 29..48
}

func TestResolveRejectsHalfSpecifiedOffsets(t *testing.T)  { /* Start set, End nil -> error */ }
func TestResolveRejectsMidRuneOffset(t *testing.T)         { /* End inside a multi-byte rune -> error */ }
func TestResolveSubstringPathUnchanged(t *testing.T)       { /* seed-style label, no offsets -> same spans as today */ }
```

- [ ] **Step 2: Verify they fail**

Run: `go test ./golden/ -run TestResolve -v`
Expected: FAIL — `Start` field undefined.

- [ ] **Step 3: Implement offset handling in `resolve`**

Keep the substring branch byte-for-byte identical; add the offset branch ahead of
it with the validation listed in Produces. Reuse `utf8` boundary checks.

- [ ] **Step 4: Run the full package and the CI gate**

Run: `go test ./golden/ -v && go test ./...`
Expected: PASS — including `TestGoldenDatasetAccuracy` with the untouched seed set.

---

### Task 4: Tool module + character→byte translation

**Risk:** high — this is the correctness core; a wrong translation produces
confidently wrong measurements rather than a visible error.

**Files:**
- Create: `tools/corpusbench/go.mod`, `tools/corpusbench/convert/offset.go`
- Test: `tools/corpusbench/convert/offset_test.go`

**Interfaces:**
- Consumes: nothing (pure function).
- Produces:
  - `func CharToByte(text string, charStart, charEnd int) (byteStart, byteEnd int, error)`
    — errors on: non-UTF-8 text, either index out of `[0, runeCount]`,
    `charStart >= charEnd`, or a boundary not landing on a rune.
  - `type Span struct { Start, End int; Label string }` (corpus char offsets).

- [ ] **Step 1: Create the tool module**

Scaffold `tools/corpusbench/go.mod` with only the module line
`module dataprotect/tools/corpusbench` and its `go` directive. **Do not add the
`require` directives here**: `CharToByte` imports neither parquet-go nor
`dataprotect`, so `go mod tidy` would strip them and Task 5 would build against a
module that lost its dependencies. The requires are added in Task 5, the first
task whose code imports both.

Verify the root cannot see the module: `cd tools/corpusbench && go build ./...`
succeeds, then from the root `go list ./...` contains no `tools/` entry.

- [ ] **Step 2: Write the failing tests**

```go
func TestCharToByteNonASCIIShiftsOffsets(t *testing.T) {
	text := "…31 dicembre 2023…"   // multi-byte runes before the target
	// want: CharToByte over the char span selects exactly "31 dicembre 2023"
}
func TestCharToByteRejectsMidRuneBoundary(t *testing.T)  { /* char index inside a rune -> error */ }
func TestCharToByteRejectsOutOfBounds(t *testing.T)      { /* charStart > runeCount -> error */ }
func TestCharToByteRejectsNonUTF8Text(t *testing.T)      { /* invalid byte -> error */ }

func FuzzCharToByte(f *testing.F) {
	// no panic; result always on rune boundaries; charIndex round-trips on valid UTF-8
}
```

- [ ] **Step 3: Verify they fail**

Run: `cd tools/corpusbench && go test ./convert/ -v`
Expected: FAIL — `CharToByte` undefined.

- [ ] **Step 4: Implement `CharToByte`**

Single pass over `text` accumulating byte position per rune, then index the
result. Keep it allocation-bounded for the fuzz corpus.

- [ ] **Step 5: Run tests including fuzz**

Run: `cd tools/corpusbench && go test ./convert/ -v -fuzz FuzzCharToByte -fuzztime 30s`
Expected: PASS; fuzz finds no panic and no mid-rune result.

---

### Task 5: Corpus adapters, label maps, fail-fast validation

**Risk:** standard — two parsers over known-good fixtures with an explicit
validation list; the hard part (offsets) already exists from Task 4.

**Files:**
- Create: `tools/corpusbench/convert/nemotron.go`, `.../gretel.go`,
  `.../literal.go`, `.../labels.go`, `.../convert.go` (assembly + `Write`)
- Test: `tools/corpusbench/convert/nemotron_test.go`, `.../gretel_test.go`,
  `.../literal_test.go`, `.../convert_test.go`

**Interfaces:**
- Consumes: `CharToByte`, `Span` (Task 4); `golden.Case`, `golden.Label`,
  `golden.Schema` (Task 1/3).
- Produces:
  - `func NemotronSpans(repr string) ([]Span, error)` — Python-repr parser.
  - `func GretelSpans(raw string) ([]Span, error)` — JSON.
  - `func BuildLabels(text string, spans []Span, labelMap map[string]recognizers.EntityType) (labels []golden.Label, unmapped []string, err error)` — translates via `CharToByte`, applies the map, collects unmapped corpus labels.
  - `func Nemotron(dataRoot string, limit int) ([]golden.Case, golden.Schema, error)`
  - `func Gretel(dataRoot string, limit int) ([]golden.Case, golden.Schema, error)`
  - `func Write(dir string, cases []golden.Case, schema golden.Schema) error`
  - label maps as package values, exactly as spec §5.5 (nemotron: `email`,
    `phone_number`, `fax_number`, `credit_debit_card`, `ssn`, `ipv4`, `ipv6`,
    `url`; gretel: `email`, `phone_number`, `credit_card_number`, `ssn`,
    `ipv4`, `ipv6`, `iban`).

- [ ] **Step 1: Add the tool module's dependencies**

In `tools/corpusbench/go.mod` add `require dataprotect v0.0.0`,
`replace dataprotect => ../..`, and parquet-go pinned to an exact version. This
is the first task whose code imports both, so `go mod tidy` keeps them. Confirm
the root module is still dependency-free and still blind to `tools/`.

Run: `cd tools/corpusbench && go build ./... && go mod tidy && cd .. && cd .. && go list ./...`
Expected: build succeeds; no `tools/` entry in `go list`; root `go.mod` still has
no `require` block.

- [ ] **Step 2: Write the tests**

```go
func TestNemotronSpansQuotedText(t *testing.T) {
	in := `[{'start': 184, 'end': 201, 'text': "bachelor's degree", 'label': 'education_level'}]`
	// want: one Span{184, 201, "education_level"} — apostrophe inside double quotes parses
}
func TestNemotronSpansNonStringText(t *testing.T)   { /* 'text': 57 -> still parses; label read */ }
func TestGretelSpansJSON(t *testing.T) {
	in := `[{"start": 131, "end": 139, "label": "employee_id"}]`
	// want: one Span{131, 139, "employee_id"}
}
func TestBuildLabelsDropsUnmappedLabels(t *testing.T)  { /* a `name` span is not a Label, but appears in unmapped */ }
// Fail-fast (spec §7.3): each aborts naming corpus, source file, row index, span index.
func TestConvertFailsFastOnOutOfBoundsSpan(t *testing.T)
func TestConvertFailsFastOnZeroLengthSpan(t *testing.T)
func TestConvertFailsFastOnMidRuneBoundary(t *testing.T)
func TestConvertFailsFastOnEmptyCaseText(t *testing.T)
func TestConvertFailsFastOnUnparseableLiteral(t *testing.T)
// Spec §7.5: overlapping gold is KEPT and counted, never a failure.
func TestConvertKeepsOverlappingGold(t *testing.T) {
	// fixture: two mapped spans that overlap
	// want: both become Labels, schema.overlapping_pairs == 1, err == nil
}
func TestConvertDeterministic(t *testing.T)  // two runs -> byte-identical output
func TestWriteEmitsSortedOutputAndSidecar(t *testing.T) // sorted files + schema.json
```

Parquet fixtures are **generated at test time by parquet-go's writer** over
hand-written synthetic records (non-ASCII record included). No corpus bytes are
committed.

- [ ] **Step 3: Verify they fail**

Run: `cd tools/corpusbench && go test ./convert/ -v`
Expected: FAIL — functions undefined.

- [ ] **Step 4: Implement parsers, maps, assembly, and `Write`**

Parsers return `[]Span`; `BuildLabels` is the only place offsets are translated
(it must not bypass `CharToByte`). Validation is a closed list from spec §5.2, and
every clause has a test above it: out-of-bounds span, zero-length span, non-rune
boundary, non-UTF-8 text, empty case text, unparseable literal, mapped label with
no `recognizers` counterparty — each aborts with corpus/file/row/span context. Records with no
in-schema label become negative cases and are **not** failures. Overlapping gold
labels are kept and counted into `overlapping_pairs`. `Write` emits the sorted
fixed-width file sequence plus `schema.json`.

- [ ] **Step 5: Run tests**

Run: `cd tools/corpusbench && go test ./... -v`
Expected: PASS.

---

### Task 6: CLI, Makefile, script wiring, documentation

**Risk:** standard — wiring and docs over already-tested components.

**Files:**
- Create: `tools/corpusbench/main.go`
- Modify: `Makefile` (`help`, new `corpus-convert`, `corpus`, `corpus-clean`);
  `scripts/benchmark.sh` (apply sidecar when present); `.env.example`; `TODO.md`;
  `AGENTS.md`; `SPEC.md`; `README.md`; `docs/concepts/piiscrub-usage.md`;
  `DATASET_LICENSES.md`

**Interfaces:**
- Consumes: `convert.Nemotron`, `convert.Gretel`, `convert.Write`;
  `golden.LoadSchema`, `golden.WithInSchema`.
- Produces: binary `corpusbench` with subcommand `convert`; Makefile targets;
  `benchmark.sh` behaviour unchanged when no sidecar is present.

- [ ] **Step 1: Implement the CLI and Makefile targets**

Positional args only: `corpusbench convert <corpus> <data-root> <limit>`.
`LIMIT ?= 2000`. `corpus-convert` runs the tool for both corpora; `corpus` runs
`corpus-convert` then `scripts/benchmark.sh` over each output dir; `corpus-clean`
removes generated output. `help` lists all three with `LIMIT`'s default.

- [ ] **Step 2: Wire the sidecar into `benchmark.sh`**

Load `schema.json` when it exists in the scored directory and pass
`golden.WithInSchema` from it; with no sidecar the generated program must be
byte-for-byte equivalent to today's behaviour.

- [ ] **Step 3: Verify the end-to-end path**

Run: `make corpus && make test`
Expected: `make corpus` converts both corpora and prints two reports (exact,
overlap, per-type, out-of-schema); `make test` passes with all pre-existing
tests unchanged. Confirm `git status` shows **no** file under `testdata/external/`.

- [ ] **Step 4: Documentation**

Close the converter entry in `TODO.md` and add entries for whatever the
measurement exposes (plus the deferred items the spec lists: BITCOIN coverage,
per-language gretel scoring, CI gating of corpus results, anonymize-over-corpus).
`AGENTS.md` gains the tool module and its isolated dependency. `SPEC.md` states
URL is nemotron-only, IBAN gretel-only, BITCOIN unmeasured, and replaces the
">99%" column with the measured number once available. `README.md` and
`docs/concepts/piiscrub-usage.md` are re-synced together, re-applying the four
link rewrites TODO.md documents. `DATASET_LICENSES.md` notes derived output stays
in the gitignored directory. `.env.example` gains `DATAPROTECT_DATA_DIR`.

- [ ] **Step 5: Final verification**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./... && make corpus`
Expected: all clean, root `go.mod` still has no `require` block, corpus reports
printed, `git status` shows only source/doc changes.
