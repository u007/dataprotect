# piiscrub — Go PII Detection & Anonymization Library

piiscrub finds personal data and secrets in text and CSV cells, and replaces them with a redaction, a mask, a hash or a numbered placeholder. It is written for inline scrubbing before LLM calls, for data pipelines and for log sanitization.

Detection uses regular expressions with checksum and rule validators: Luhn for cards, mod-97 for IBANs, Base58Check and bech32 for Bitcoin addresses, and check digits for the national IDs. The library uses only the Go standard library.

## What it is not

piiscrub has no language model and no named-entity recognition. It does not detect person names, organisation names, street addresses or other free-text entities. An NLP recognizer is planned for v2 in [SPEC.md](SPEC.md) and does not exist yet.

As implemented, the scope is:

- It detects the 14 entity types listed under "Entity types" below.
- Malaysia and Singapore passports are matched only next to a passport keyword. Thai passports are not covered.
- Bare domains such as `microsoft.com` are not detected. Phone numbers in national formats (Israel, Japan, UK) are not matched.
- It is not a formal privacy tool. It gives no k-anonymity or differential-privacy guarantee, and it handles English text only (SPEC.md, "Non-goals").

## Setup

### Requirements

- Go 1.24 or newer. The root `go.mod` declares `go 1.24`.
- Corpus work only: `tools/corpusbench` declares `go 1.24.9`. `make presidio` needs `git` and `python3`. `make download` needs the `hf` CLI (`pip install -U huggingface_hub`) and about 450 MB of disk, the figure in the Makefile help.

### Install

```bash
go get github.com/u007/dataprotect
```

The module path is `github.com/u007/dataprotect`, from the root `go.mod`, and it matches the `origin` remote. The repository has no git tags, and this README did not check that `go get` resolves against the public remote. For a local checkout, point the consuming module at the directory:

```
require github.com/u007/dataprotect v0.0.0
replace github.com/u007/dataprotect => /path/to/dataprotect
```

The import path ends in `dataprotect`, but the package name is `piiscrub`. Import it without an alias and refer to it as `piiscrub`.

### Quick start

`Scrubber` wires every default recognizer and operator. Its confidence threshold is 0.7, and context boosting is off.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/u007/dataprotect"
)

func main() {
	ctx := context.Background()
	s := piiscrub.NewScrubber()

	out, err := s.Anonymize(ctx, "SSN 123-45-6789")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(out)
	// Output: SSN ***-**-6789

	entities, err := s.Analyze(ctx, "mail a@b.com")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(entities[0].Type)
	// Output: EMAIL

	redacted, pseudonyms, err := s.Pseudonymize(ctx, "mail a@b.com and a@b.com")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(redacted)
	// Output: mail [EMAIL_1] and [EMAIL_1]
	fmt.Println(pseudonyms)
	// Output: map[[EMAIL_1]:a@b.com]
}
```

`Pseudonymize` replaces each value with a numbered placeholder and returns the map needed to restore it. The map holds the original values in plain text, so protect it as you would the input.

### Building the pipeline by hand

Use `analyzer` and `anonymizer` directly to choose recognizers and operators yourself:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/u007/dataprotect/analyzer"
	"github.com/u007/dataprotect/anonymizer"
	"github.com/u007/dataprotect/operators"
	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/checksum"
	"github.com/u007/dataprotect/recognizers/regex"
)

func main() {
	ctx := context.Background()

	// Cards are detected by the Luhn validator, not by the regex card
	// recognizer. The regex matches any 13-19 digit run, so a number that fails
	// the checksum would be reported and masked as payment data.
	a := analyzer.New(analyzer.WithMinConfidence(0.7))
	a.AddRecognizer(regex.NewEmailRecognizer())
	a.AddRecognizer(regex.NewPhoneRecognizer())
	a.AddRecognizer(checksum.NewLuhnValidator())

	an := anonymizer.New(anonymizer.WithDefaultOperator(operators.NewRedactOperator()))
	an.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	an.SetOperator(recognizers.EntityPhone, operators.NewTailMaskOperator("***-***-####"))
	an.SetOperator(recognizers.EntityCreditCard, operators.NewTailMaskOperator("****-****-****-####"))

	text := "Contact John at john@example.com or 555-123-4567. Card: 4111111111111111."
	entities, err := a.Analyze(ctx, text)
	if err != nil {
		log.Fatal(err)
	}
	result, err := an.Anonymize(ctx, text, entities)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
	// Output: Contact John at <EMAIL> or ***-***-4567. Card: ****-****-****-1111.
}
```

Recognizers are added with `AddRecognizer`. Each one reports entities, and `Analyze` resolves overlaps before it returns.

### Entity types

These are the 14 types in `recognizers.AllEntityTypes`. The "Scrubber" column is the operator `NewScrubber` applies by default.

| Type | Detected by | Method and limits | Scrubber operator |
|---|---|---|---|
| `EMAIL` | `regex.NewEmailRecognizer` | Regex (RFC 5322 simplified), Unicode and punycode domains | replace `<EMAIL>` |
| `PHONE` | `regex.NewPhoneRecognizer` | Regex, US and international forms, parenthesised area codes | tail mask `***-***-####` |
| `CREDIT_CARD` | `checksum.NewLuhnValidator` | Luhn checksum; the regex card recognizer exists but is not wired by default | tail mask `****-****-****-####` |
| `SSN` | `regex.NewSSNRecognizer` | Shape only (`NNN-NN-NNNN` with separators); a bare nine-digit value is accepted only as the whole input | mask `***-**-####` |
| `IP_ADDRESS` | `regex.NewIPRecognizer` | IPv4, and IPv6 candidates validated by `netip` | redact |
| `URL` | `regex.NewURLRecognizer` | HTTP and HTTPS, trailing punctuation trimmed | redact |
| `IBAN` | `checksum.NewIBANValidator` | Pattern, mod-97 and per-country length table | redact |
| `BITCOIN` | `regex.NewBitcoinRecognizer` | P2PKH, P2SH and bech32 (`bc1`, including Taproot), accepted only when the Base58Check or bech32 checksum verifies | redact |
| `SECRET` | `secret.NewSecretRecognizer` | API keys, tokens, passwords, PEM private key headers, and provider tokens with no label (AWS, GitHub, Slack bot and user, Stripe live, Google, npm, SendGrid, bare JWT); see "Known limits" | redact |
| `SG_NRIC` | `national.NewSGNRICRecognizer` | Prefixes S, T, F and G, with the check letter. The M prefix is not supported | redact |
| `MY_IC` | `national.NewMYICRecognizer` | MyKad, hyphenated form, date of birth check; MyKad has no checksum | redact |
| `TH_ID` | `national.NewTHIDRecognizer` | Thai citizen ID, 13 digits with the mod-11 check digit. A Luhn-valid Thai ID is typed `TH_ID` by `Scrubber` | redact |
| `MY_PASSPORT` | `national.NewMYPassportRecognizer` | A, H or K and eight digits, next to a passport keyword | redact |
| `SG_PASSPORT` | `national.NewSGPassportRecognizer` | E or K, seven digits and a letter, next to a passport keyword | redact |

The shapes for the national IDs and passports come from the sources in [SOURCES.md](SOURCES.md). The Malaysian and Singapore passport shapes are not confirmed by an official specification.

### Operators

| Operator | Constructor | Example |
|---|---|---|
| replace | `operators.NewReplaceOperator("<EMAIL>")` | any value becomes `<EMAIL>` |
| mask | `operators.NewMaskOperator("***-**-####")` | `123-45-6789` becomes `***-**-6789`; `#` keeps a character, `*` masks it, front-aligned |
| tail mask | `operators.NewTailMaskOperator("***-***-####")` | `555-123-4567` becomes `***-***-4567`; a value longer than the pattern is aligned to the end |
| hash | `operators.NewHashOperator(12)` | SHA-256 hex of the value, truncated to 12 characters (`0` keeps all 64); the same value gives the same hash |
| redact | `operators.NewRedactOperator()` | the value becomes `[REDACTED]` |

Pseudonymization is not an operator. It is `Anonymizer.Pseudonymize`, or `Scrubber.Pseudonymize` and `Scrubber.PseudonymizeAll`, and it ignores the configured operators. Numbering is local to each call, so a caller that joins separately pseudonymized texts must restore each one before joining. See SPEC.md, "Core Interfaces".

### Options

- `analyzer.WithMinConfidence(t)` keeps an entity when its confidence is at least `t`. The threshold is inclusive. `Scrubber` uses 0.7.
- `analyzer.WithContextBoost()` raises confidence from surrounding words. It is opt-in, because it changes overlap outcomes. It runs before the threshold and before overlap resolution, and it never lifts a span that overlaps a URL which survives the threshold. `Scrubber` does not enable it; build an `Analyzer` yourself to use it. On the two corpora, exact precision and recall were identical with boosting on and off (SPEC.md, "Core Interfaces" and the corpus measurement).
- `anonymizer.WithDefaultOperator(op)` sets the operator for types with no `SetOperator` entry. Without it, an entity whose type has no operator is an error. It is never returned unredacted.

### Structured data (CSV)

`structured` processes CSV files. `Scrubber` has no CSV method, so build the analyzer and anonymizer as above and pass them to `structured.NewBatch`:

```go
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/u007/dataprotect/analyzer"
	"github.com/u007/dataprotect/anonymizer"
	"github.com/u007/dataprotect/operators"
	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/checksum"
	"github.com/u007/dataprotect/recognizers/regex"
	"github.com/u007/dataprotect/structured"
)

func main() {
	ctx := context.Background()

	a := analyzer.New(analyzer.WithMinConfidence(0.7))
	a.AddRecognizer(regex.NewEmailRecognizer())
	a.AddRecognizer(checksum.NewLuhnValidator())

	an := anonymizer.New(anonymizer.WithDefaultOperator(operators.NewRedactOperator()))
	an.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))

	bp := structured.NewBatch(a, an)
	bp.SetColumnType("email", recognizers.EntityEmail)
	bp.SetColumnType("note", recognizers.EntityCreditCard)

	in := "id,email,note\n1,a@b.com,card 4111111111111111\n2,c@d.com,short\n3,e@f.com\n"

	var out bytes.Buffer
	ragged, err := bp.ProcessCSV(ctx, bytes.NewBufferString(in), &out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out.String())
	fmt.Println("ragged rows:", ragged)
	// Output:
	// id,email,note
	// 1,<EMAIL>,card [REDACTED]
	// 2,<EMAIL>,short
	// 3,<EMAIL>,
	// ragged rows: 1

	var out2 bytes.Buffer
	n, pseudonyms, err := bp.PseudonymizeCSV(ctx, bytes.NewBufferString(in), &out2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out2.String())
	fmt.Println("ragged rows:", n, "mapping size:", len(pseudonyms))
	// Output:
	// id,email,note
	// 1,[EMAIL_1],card [CREDIT_CARD_1]
	// 2,[EMAIL_2],short
	// 3,[EMAIL_3],
	// ragged rows: 1 mapping size: 4
}
```

- Only columns registered with `SetColumnType` are scanned. Header cells are scanned like data cells.
- A short row is padded with empty cells. A long row keeps its extra cells, and they are not scanned. The returned count is the number of rows whose field count differs from the header, so a caller can detect ragged input.
- `PseudonymizeCSV` uses one mapping for the whole file. The map holds raw PII.

## Development

```bash
go build ./...
go vet ./...
go test ./...            # the library suite (make test)
go test -v ./recognizers/...
make fmt                 # fails if any file is not gofmt-clean
```

Make targets (from `make help`):

| Target | What it does |
|---|---|
| `build`, `vet`, `test`, `fmt` | build every package, run `go vet`, run the library suite, check gofmt |
| `test-tools` | run the `tools/corpusbench` suite (separate module) |
| `benchmark` | score `testdata/golden` and print the report (`scripts/benchmark.sh`) |
| `list`, `info NAME=<dataset>`, `dry-run`, `download` | licence and fetch helpers (`scripts/fetch-dataset.sh`); `download` fetches the commercially usable corpora |
| `corpus-convert`, `corpus` | convert the corpora to golden cases (`LIMIT`, default 2000); `corpus` also scores them |
| `corpus-anonymize` | run `Scrubber.Anonymize` and the pseudonymize round trip over every converted case |
| `corpus-clean`, `clean-data` | delete the converted datasets, or all of `testdata/external` |
| `presidio-fetch`, `presidio` | clone Microsoft Presidio at the pinned revision, extract its test cases, and score them |

Module layout:

- The root module is the library. Its `go.mod` has no `require` block and must stay that way.
- `tools/corpusbench` is a separate module with its own `go.mod`. It holds the Parquet reader and imports the root through `replace github.com/u007/dataprotect => ../..`. `go test ./...` at the root does not run its tests; use `make test-tools`.
- `golden/accuracy_test.go` is the CI gate. It scores `testdata/golden/seed.json`. The corpora are gitignored, so CI cannot see them.

Packages: `analyzer/` (engine and overlap resolution), `anonymizer/` (operators and pseudonymization), `operators/`, `recognizers/` (`regex`, `checksum`, `national`, `secret`), `structured/` (CSV), `internal/context/` (context boosting), `golden/` (scoring), `tools/corpusbench/` (corpus conversion), `scripts/`, and `testdata/golden/`.

## Datasets and measured results

Measured on 2026-10-19 with `make benchmark`, `make corpus` and `make presidio`, run on the working tree at commit `9698b27` with uncommitted changes. The seed set is 549 cases; the committed seed at `9698b27` is 537. Every number in this section comes from those runs or is copied from [SPEC.md](SPEC.md) and [SOURCES.md](SOURCES.md).

### Sources

Corpora are fetched into `testdata/external/` (gitignored). Set `DATAPROTECT_DATA_DIR` to keep them elsewhere; the fetch script and the Makefile both read it. Each fetched directory holds a `PROVENANCE.txt` with the revision, and each converted directory holds `schema.json`, which records the input's SHA-256 and the row bound.

| Dataset | Source | Pinned revision | Licence | Rows used | Local path | How to fetch |
|---|---|---|---|---|---|---|
| Golden seed set | `testdata/golden/seed.json`, authored in this repository from synthetic text | none; the working tree (see above) | none needed, authored here (DATASET_LICENSES.md, "Provenance") | 549 cases: 350 positive, 199 negative | `testdata/golden/` (committed) | nothing to fetch |
| Nemotron-PII | [nvidia/Nemotron-PII](https://huggingface.co/datasets/nvidia/Nemotron-PII) | `b70ffaf5ff39e079776134c5bf4381f00a9fd1ed` | CC-BY-4.0 (attribution) | 2000 of 100000 rows, test split, file order | `testdata/external/nemotron-pii/` | `scripts/fetch-dataset.sh fetch nemotron-pii`, or `make download` |
| Gretel synthetic PII finance (multilingual) | [gretelai/synthetic_pii_finance_multilingual](https://huggingface.co/datasets/gretelai/synthetic_pii_finance_multilingual) | `7b844d16738527a04264f50214cb426a4cea0897` | Apache-2.0 with NOTICE (attribution) | 2000 of 5596 rows, `combined_test` split only | `testdata/external/gretel-pii-finance/` | `scripts/fetch-dataset.sh fetch gretel-pii-finance`, or `make download` |
| Microsoft Presidio test cases | [microsoft/presidio](https://github.com/microsoft/presidio) | `2523c7b74a469270c5c78bb253f140eafca21e31` | MIT | 590 cases from 601 rows, ten modules; 11 rows set aside | `testdata/external/presidio/` | `make presidio` (clones at the pin and extracts with `scripts/presidio_extract.py`) |

`make download` skips the restricted corpora. For those, see "Datasets considered and not used" below.

### Headline results

| Dataset | Cases | Match | Precision | Recall | F1 | tp / fp / fn |
|---|---|---|---|---|---|---|
| Golden seed | 549 | exact span | 1.0000 | 0.9974 | 0.9987 | 381 / 0 / 1 |
| Golden seed | 549 | byte overlap | 1.0000 | 0.9974 | 0.9987 | 381 / 0 / 1 |
| Nemotron-PII | 2000 | exact span | 0.8357 | 0.8916 | 0.8627 | 2426 / 477 / 295 |
| Nemotron-PII | 2000 | byte overlap | 0.8384 | 0.8945 | 0.8656 | 2434 / 469 / 287 |
| Gretel PII finance | 2000 | exact span | 0.3677 | 0.8201 | 0.5078 | 652 / 1121 / 143 |
| Gretel PII finance | 2000 | byte overlap | 0.3869 | 0.8629 | 0.5343 | 686 / 1087 / 109 |

Negative cases with no detection: seed 199 of 199; Nemotron-PII 472 of 600; gretel 1291 of 1497.

The seed set is hand-written and small. Its numbers are a floor, not a result. The corpus numbers are real measurements of a 2000-record sample, not a target. The seed set is the only one used as a CI gate.

### Per type

Exact span, per type. A dash means the corpus schema does not label that type. Detections of such a type are reported as out of schema and are not scored: Nemotron-PII has IBAN 1 and SECRET 63 out of schema; gretel has BITCOIN 21, MY_PASSPORT 1, SECRET 25 and URL 817. The seed column gives case counts from `testdata/golden/seed.json` and gold spans from the benchmark.

| Type | Seed cases (gold spans) | Seed P / R (tp / fp / fn) | Nemotron-PII P / R | Gretel P / R |
|---|---|---|---|---|
| `EMAIL` | 51 (57) | 1.0000 / 1.0000 (57 / 0 / 0) | 0.9975 / 0.9975 | 0.3281 / 0.8060 |
| `PHONE` | 54 (58) | 1.0000 / 0.9828 (57 / 0 / 1) | 0.5703 / 0.9982 | 0.2766 / 0.8179 |
| `CREDIT_CARD` | 49 (56) | 1.0000 / 1.0000 (56 / 0 / 0) | 0.5333 / 0.0945 | 0.5094 / 0.4737 |
| `SSN` | 32 (36) | 1.0000 / 1.0000 (36 / 0 / 0) | 0.8780 / 1.0000 | 0.9787 / 0.8846 |
| `IP_ADDRESS` | 44 (46) | 1.0000 / 1.0000 (46 / 0 / 0) | 0.9828 / 0.9771 | 0.8400 / 0.9767 |
| `URL` | 30 (30) | 1.0000 / 1.0000 (30 / 0 / 0) | 0.9900 / 0.9229 | - |
| `IBAN` | 35 (35) | 1.0000 / 1.0000 (35 / 0 / 0) | - | 0.9556 / 0.9773 |
| `BITCOIN` | 13 (13) | 1.0000 / 1.0000 (13 / 0 / 0) | - | - |
| `SECRET` | 34 (34) | 1.0000 / 1.0000 (34 / 0 / 0) | - | - |
| `SG_NRIC` | 5 (5) | 1.0000 / 1.0000 (5 / 0 / 0) | - | - |
| `MY_IC` | 3 (3) | 1.0000 / 1.0000 (3 / 0 / 0) | - | - |
| `TH_ID` | 4 (4) | 1.0000 / 1.0000 (4 / 0 / 0) | - | - |
| `MY_PASSPORT` | 3 (3) | 1.0000 / 1.0000 (3 / 0 / 0) | - | - |
| `SG_PASSPORT` | 2 (2) | 1.0000 / 1.0000 (2 / 0 / 0) | - | - |

Upper bounds. The benchmark also reports precision with the set-aside detections left out. Nemotron-PII exact precision is 0.9914 (456 detections set aside) and overlap precision 0.9947. Gretel exact precision is 0.3717 (19 set aside) and overlap precision 0.3911. An upper bound is not accuracy. It assumes that every set-aside detection is correct, and the set-aside spans are the ones the corpus labels with a name we do not map to a type, so some of them could be real errors. SPEC.md says the Nemotron-PII PHONE bound should not be quoted as meaningful: 408 of its set-aside detections sit on ID-type fields that the bare 10-digit phone branch matches. They are PII under the corpus labels and the scrubber redacts them, so this is a label-scope artefact, not a detector error.

### Why gretel precision is low

Most of the gretel false positives are two types: EMAIL (383) and PHONE (693), out of 1121. For EMAIL, SPEC.md attributes the gap to the corpus. Some records label only the local part as `user_name`. A Markdown `mailto:` link renders the address twice while the corpus labels it once. In records that carry any email label, only 106 of 647 email-shaped strings are labelled `email`. For PHONE, 679 of the 693 gretel false positives sit on digit runs the corpus leaves unlabelled: EDIFACT, SWIFT, XML and CSV values, placeholders such as `1234567890`, and ID fields. The bare 10-digit branch of the phone pattern matches them. About 40 are real-looking phone numbers the corpus missed, so the true precision is higher than measured. No narrowing rule was adopted, because each candidate either drops a gold phone or stops redacting numbers that could be phones (TODO.md, 2026-10-19). The EMAIL attribution is also still open: TODO.md keeps "Corpus-side label quality depresses measured precision" unchecked.

### Presidio test cases

The Presidio measurement covers the ten modules whose entity type this library implements. Each module is scored as its own dataset, so the schema is that module's type. Exact span counts are `(tp, fp, fn)`. The overlap column counts a span that touches a gold span as a match, so it is an upper bound.

| Module | Type | Cases | Negatives | Exact P / R (tp, fp, fn) | Overlap P / R |
|---|---|---|---|---|---|
| `email` | EMAIL | 8 | 2 | 1.0000 / 1.0000 (7, 0, 0) | 1.0000 / 1.0000 |
| `ip` | IP_ADDRESS | 112 | 29 | 0.8049 / 0.7500 (66, 16, 22) | 1.0000 / 0.9318 |
| `url` | URL | 16 | 3 | 0.7500 / 0.6429 (9, 3, 5) | 0.9167 / 0.7857 |
| `iban` | IBAN | 330 | 173 | 1.0000 / 1.0000 (158, 0, 0) | 1.0000 / 1.0000 |
| `crypto` | BITCOIN | 9 | 3 | 1.0000 / 1.0000 (7, 0, 0) | 1.0000 / 1.0000 |
| `credit_card` | CREDIT_CARD | 19 | 5 | 0.9412 / 1.0000 (16, 1, 0) | 0.9412 / 1.0000 |
| `phone` | PHONE | 19 | 0 | 0.7917 / 0.6786 (19, 5, 9) | 1.0000 / 0.8571 |
| `us_ssn` | SSN | 28 | 12 | 0.5769 / 0.8824 (15, 11, 2) | 0.5769 / 0.8824 |
| `th_tnin` | TH_ID | 39 | 26 | 0.8125 / 1.0000 (13, 3, 0) | 0.8125 / 1.0000 |
| `sg_fin` | SG_NRIC | 10 | 1 | 1.0000 / 0.5000 (5, 0, 5) | 1.0000 / 0.5000 |

Totals: 590 cases from 601 rows. 11 rows are set aside: 3 IP count-only rows, 6 phone leniency rows and 2 empty-text negatives (SPEC.md, "Presidio test cases"). For `ip`, SPEC.md explains the exact-span misses as span conventions: CIDR suffixes, zone IDs, and the unspecified `::`, which is not redacted by design. Phone national formats are out of scope. Neither SPEC.md nor TODO.md analyses the `url` misses. The five `sg_fin` misses were checked by running the recognizer on each value: `M4332674T` has an unsupported M prefix; `A1234567Z` and `B1234567Z` have prefixes outside S, T, F and G; and `T1234567Z` and `G1122144L` fail the recognizer's check letter. Presidio's gold labels all five.

## Datasets considered and not used

Copied from [SOURCES.md](SOURCES.md) and [DATASET_LICENSES.md](DATASET_LICENSES.md). Where no URL is recorded, none is given here.

| Dataset | Link | Licence issue | Status |
|---|---|---|---|
| CoNLL-2003 (`eriktks/conll2003`) | [huggingface.co/datasets/eriktks/conll2003](https://huggingface.co/datasets/eriktks/conll2003) | Built on the Reuters RCV1 corpus, which is not freely redistributable. Tagged `license: other` | Evaluation only. Not fetched |
| ai4privacy `pii-masking-43k` | [huggingface.co/datasets/ai4privacy/pii-masking-43k](https://huggingface.co/datasets/ai4privacy/pii-masking-43k) | Custom dual-tier licence, not CC-BY-SA. Free only for individuals, non-profits, for-profits with 3 or fewer staff, or evaluation. Derivatives cannot be sublicensed | Avoid for commercial use. Not fetched |
| ai4privacy `openpii-1m` | [huggingface.co/datasets/ai4privacy/pii-masking-openpii-1m](https://huggingface.co/datasets/ai4privacy/pii-masking-openpii-1m) | The licence tag (`other`) and the declared `cc-by-4.0` disagree | Read the LICENCE file before any use. Not fetched |
| PIIBench (`Pritesh-2711/pii-bench`) | [huggingface.co/datasets/Pritesh-2711/pii-bench](https://huggingface.co/datasets/Pritesh-2711/pii-bench) | Declared apache-2.0, but it consolidates ten datasets, including CoNLL-2003 and ai4privacy, and their licences govern | Audit before use. Not fetched |
| OpenNER 1.0 | [github.com/bltlab/open-ner](https://github.com/bltlab/open-ner) | CC-BY 4.0 compilation. Its constituent corpora keep their own licences, some of which restrict commercial use | Not fetched or used. Licence review only |
| Kaggle PII Detection | no URL recorded | Per-competition terms, commonly forbidding redistribution | Not located. Not fetched |
| Financial PII (Mendeley) | no URL recorded | Licence not verified | Not located. Not fetched |
| SecretBench | no URL recorded (SPEC.md names arXiv) | No licence recorded | Not located. Not fetched |
| secrets-patterns-db | no URL recorded (SPEC.md names GitHub) | No licence recorded | Not located. Not fetched |
| CoNLL 2025 NER | no URL recorded (SPEC.md names Hugging Face) | Open question; the registry pins `conll2003` instead | Undecided |

The secret recognizer has no external corpus. Its only scored data is the 34 SECRET cases in the seed set. The corpora annotate some secrets under labels outside the type map (gretel `api_key`; nemotron `api_key` and `http_cookie`). Those spans are unlabelled, not scored, so their SECRET detections are out of schema and not scored. Nothing from the Kaggle competition is used yet.

## Known limits

Each item links to its open entry in [TODO.md](TODO.md) or to the rule in [SPEC.md](SPEC.md).

- **SSN is shape-only.** `666-12-3456` is reported, because Presidio's never-issued rules are not applied (TODO.md, "SSN: shape-only").
- **Phone national formats.** Israel, Japan and UK national formats are not matched (TODO.md, "Phone: national formats").
- **Truncated international numbers.** `+55 11 98456 5666` is reported as `+55 11 98456`, so `5666` stays visible. `+33 1 42 68 53 00` is reported as `+33 1 42 68`. This was accepted as a user decision on 2026-10-16 (TODO.md, "Phone: truncated international numbers").
- **Bare domains.** `microsoft.com` is not detected (TODO.md, "URL: bare domains").
- **Merged card spans.** A span merged by the card-window search can cover two cards, or a card and a year. Its one placeholder restores as one original (TODO.md, "Merged card spans are one placeholder").
- **Secret over-redaction.** A plain word of eight or more characters after a label is reported: `Password: Required` anonymizes to `Password: [REDACTED]`. Uniform values such as `00000000000000000000` are not reported. Dotted tails are kept or trimmed by a fixed rule (SPEC.md, "Recognizers", secret row).
- **Label boundary, accepted cost.** `pass` and `pwd` need a non-letter before them, so `bypass=` and `compass:` are not reported. The same rule means `dbpass=x` and `rootpwd=x` are not reported either (TODO.md, secret section, 2026-10-19).
- **Closing bracket after a password.** One trailing `)`, `]` or `}` is dropped when whitespace or the end of text follows it and the value has no opener for it, so `(pass=Hunter2xyz)` reports `Hunter2xyz`. A closing bracket followed by a quote (`"Hunter2xyz)"`) is kept and over-redacts one byte; it does not leak the value (TODO.md, secret section, 2026-10-19).
- **Provider tokens with no label, limits.** The formats in the SECRET row are matched by fixed prefix and shape only, so a token that does not follow the exact length is not reported (a GitHub token one character short is missed). Not covered: Slack `xoxa-`, `xoxr-` and `xoxs-` (no provider source found; see TODO.md), bare JWE, a bare JWT with a fourth segment, Stripe test keys (`sk_test_`), Stripe `sk_org_`, and the formats not listed. A token inside a URL is redacted as part of the URL, since the URL recognizer scores higher (0.99 against 0.98). Stated limits of the source-backed formats are in SOURCES.md.
- **Other open items.** Singapore M-prefix NRIC is not supported. Thai passports are not covered. BITCOIN is not labelled by either corpus, so it is unmeasured there. On Nemotron-PII, all 230 CREDIT_CARD false negatives fail the Luhn check. The corpus cards have valid network prefixes but random check digits (24 of 254 pass Luhn). Detecting them would mean reporting Luhn-invalid numbers, which the Luhn-only policy and the seed negatives rule out; the decision is open (SPEC.md, "Corpus measurement").

## License

piiscrub is released under the MIT licence; see [LICENSE](LICENSE).

The corpora named above are not covered by that licence and are not redistributed here. Each carries its own terms, recorded in [DATASET_LICENSES.md](DATASET_LICENSES.md). Nemotron-PII (CC-BY-4.0) and the gretel corpus (Apache-2.0 with NOTICE) require attribution. When you publish a result, cite the dataset and its revision, as the tables above do. Do not copy corpus content into `testdata/golden/`.

## More

- [SPEC.md](SPEC.md): architecture, recognizer rules, corpus measurement and Presidio measurement.
- [SOURCES.md](SOURCES.md): where each dataset, rule and reference came from, and when it was checked.
- [TODO.md](TODO.md): open items and their decisions.
- [AGENTS.md](AGENTS.md): development conventions, including threshold semantics and the module split.
- [DATASET_LICENSES.md](DATASET_LICENSES.md): dataset terms and how they relate to this repository's licence.
