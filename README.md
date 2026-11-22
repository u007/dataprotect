# piiscrub — Go PII Detection & Anonymization Library

piiscrub finds personal data and secrets in text and CSV cells, and replaces them with a redaction, a mask, a hash or a numbered placeholder. It is written for inline scrubbing before LLM calls, for data pipelines and for log sanitization.

Detection uses regular expressions with checksum and rule validators: Luhn for cards, mod-97 for IBANs, Base58Check and bech32 for Bitcoin addresses, and check digits for the national IDs. The library uses only the Go standard library.

## What it is not

piiscrub has no language model of its own. Detection is regex and checksum based; the only exception is the optional NLP sidecar (`sidecar/`, a separate module), which adds person, organisation and location detection through a separate process. The root library stays standard-library-only and the NLP recognizer is opt-in, so a caller that does not start the sidecar gets the same deterministic behaviour as before.

As implemented, the scope is:

- It detects the entity types listed under "Entity types" below. `CUSTOM_TERM` is reported only for terms the caller supplies.
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

These are the 38 types in `recognizers.AllEntityTypes`. `CUSTOM_TERM` has no recognizer in the default pipeline; see "Caller-supplied terms" below. The "Scrubber" column is the operator `NewScrubber` applies by default.

| Type | Detected by | Method and limits | Scrubber operator |
|---|---|---|---|
| `EMAIL` | `regex.NewEmailRecognizer` | Regex (RFC 5322 simplified), Unicode and punycode domains | replace `<EMAIL>` |
| `PHONE` | `regex.NewPhoneRecognizer` | Regex, US and international forms, parenthesised area codes | tail mask `***-***-####` |
| `CREDIT_CARD` | `checksum.NewLuhnValidator` | Luhn checksum at 0.99; a Luhn-failing number at 0.85 when a label-style keyword names it ("card number", "credit card", "Visa:") and the prefix and layout are plausible. The regex card recognizer exists but is not wired by default | tail mask `****-****-****-####` |
| `PERSON` | `recognizers/nlp.NLPRecognizer` (sidecar) | NER via sidecar (prose); English only | redact |
| `ORGANIZATION` | `recognizers/nlp.NLPRecognizer` (sidecar) | NER via sidecar (prose); English only | redact |
| `LOCATION` | `recognizers/nlp.NLPRecognizer` (sidecar) | NER via sidecar (prose); English only | redact |
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
| `PH_TIN` | `national.NewPHTINRecognizer` | Philippine TIN, nine digits in three groups with an optional three-digit branch. The NIRC §236 checksum formula is not verified | redact |
| `IN_GSTIN` | `national.NewINGSTINRecognizer` | Indian GSTIN, 15-character shape with the mod-36 check digit; the primary GSTN document is unconfirmed | redact |
| `SWIFT_BIC` | `finance.NewSWIFTBICRecognizer` | Keyword-gated (SWIFT, BIC, SWIFT/BIC, SWIFT code, BIC code). ISO 9362 shape, 8 or 11 characters, uppercase. The country code is any two letters, so non-IBAN countries such as US are accepted. Scores 0.85 | redact |
| `US_ROUTING` | `finance.NewUSRoutingRecognizer` | Keyword-gated (routing number, routing no, routing transit number, ABA, RTN). Nine digits, ABA checksum and an assigned prefix range. Scores 0.85 | redact |
| `CARD_CVV` | `finance.NewCardCVVRecognizer` | Keyword-gated (CVV, CVV2, CVC, CVC2, CID, CSC, CAV, security code, card verification). Three or four digits with no digit adjacent. Scores 0.85 | redact |
| `BANK_ACCOUNT` | `finance.NewBankAccountRecognizer` | Keyword-gated (account number, account no, acct number, bank account, BBAN). No universal shape: up to four leading letters, then 8 to 34 characters of digits and hyphens. Scores 0.85 | redact |
| `PIN` | `finance.NewPINRecognizer` | Keyword-gated (PIN). Four, six or eight digits, the lengths seen in the corpora. Scores 0.85 | redact |
| `TAX_ID` | `identity.NewTaxIDRecognizer` | Keyword-gated (tax id, TIN, ITIN, EIN, employer identification number, taxpayer id). EIN with an IRS-assigned prefix, or NNN-NN-NNNN, or a letter and eight digits. Scores 0.85; in default output an NNN-NN-NNNN value is reported as SSN | redact |
| `PASSPORT` | `identity.NewPassportRecognizer` | Keyword-gated (passport, passport number, passport no). Nine digits or a letter and eight digits. Loses to MY_PASSPORT and SG_PASSPORT. Scores 0.85 | redact |
| `DRIVER_LICENSE` | `identity.NewDriverLicenseRecognizer` | Keyword-gated (driver's license, driving licence, DL number). Optional letters, then digits and hyphens. Scores 0.85 | redact |
| `DATE_OF_BIRTH` | `identity.NewDateOfBirthRecognizer` | Keyword-gated (date of birth, DOB, birth date, born on). ISO, dd/mm/yyyy, or a month-name date. Reports the date only. Scores 0.85 | redact |
| `LEI` | `finance.NewLEIRecognizer` | ISO 17442 Legal Entity Identifier: 18 alphanumerics and two digits, MOD-97-10 check. No keyword; scores 0.90 | redact |
| `ISIN` | `finance.NewISINRecognizer` | Keyword-gated (ISIN). 12 characters, two letters, nine alphanumerics, Luhn check digit. Scores 0.85 | redact |
| `ETHEREUM` | `finance.NewEthereumRecognizer` | `0x` plus 40 hex characters. A mixed-case address must pass the EIP-55 Keccak-256 checksum; all-lowercase or all-uppercase is accepted by shape. Scores 0.95 | redact |
| `IN_PAN` | `national.NewINPANRecognizer` | Keyword-gated (PAN, permanent account number). Ten characters with a holder type in the fourth position. No check formula is published. Scores 0.85 | redact |
| `SG_UEN` | `national.NewSGUENRecognizer` | Keyword-gated (UEN, unique entity number, business or company registration no). Business, local company and Others forms. The check letter is not published, so shape only. Scores 0.85 | redact |
| `MY_SSM` | `national.NewMYSSMRecognizer` | Keyword-gated (registration no, company no, SSM, no. syarikat, ROC). 12 digits, or the old form of 1 to 7 digits, a hyphen and a letter. Scores 0.85 | redact |
| `MY_TIN` | `national.NewMYTINRecognizer` | Keyword-gated (TIN, tax identification number, no. cukai). `IG` plus 9 to 11 digits, or a non-individual code (C, D, E, F and others) plus 9 to 11 digits. Scores 0.85 | redact |
| `CASE_NUMBER` | `legal.NewCaseNumberRecognizer` | US federal shapes (`23-CV-0001-JFH`, `3:00CR0001-01`), Singapore court forms (`CA/CA 1/2021`, `DC / OC 1 / 2022`, `SCT/10001/2022`) at 0.90, and a label-gated generic form (case no, suit no, guaman no, and others) at 0.85. Neutral citations such as `[2021] SGHC 123` are not matched. Reports the number only | redact |
| `MONEY` | `finance.NewMoneyRecognizer` | A currency symbol or ISO 4217 code next to an amount (`$1,250.00`, `S$40`, `RM 900`, `EUR 3.5m`, `12,000 USD`). A bare number is never MONEY. Scores 0.85 and loses every overlap | redact |
| `CUSTOM_TERM` | `terms.New` (caller-supplied) | Terms the caller lists (client names, matter codes). Case-insensitive, whole-term, whitespace-tolerant. Scores 1.00 and wins every overlap. Not in the default pipeline | redact |

The shapes for the national IDs and passports come from the sources in [SOURCES.md](SOURCES.md). The Malaysian and Singapore passport shapes are not confirmed by an official specification.

### Operators

| Operator | Constructor | Example |
|---|---|---|
| replace | `operators.NewReplaceOperator("<EMAIL>")` | any value becomes `<EMAIL>` |
| mask | `operators.NewMaskOperator("***-**-####")` | `123-45-6789` becomes `***-**-6789`; `#` keeps a character, `*` masks it, front-aligned |
| tail mask | `operators.NewTailMaskOperator("***-***-####")` | `555-123-4567` becomes `***-***-4567`; a value longer than the pattern is aligned to the end |
| hash | `operators.NewHashOperator(12)` | SHA-256 hex of the value, truncated to 12 characters (`0` keeps all 64); the same value gives the same hash |
| redact | `operators.NewRedactOperator()` | redacted through `Anonymize`/`Scrubber.Anonymize` the value becomes a typed numbered marker such as `[[redactedperson1]]` or `[[redactedsocialnumber2]]`, numbered per type in order of appearance (`SSN` renders as `socialnumber`); `RedactOperator.Replace` called directly still returns `[REDACTED]` |

Pseudonymization is not an operator. It is `Anonymizer.Pseudonymize`, or `Scrubber.Pseudonymize` and `Scrubber.PseudonymizeAll`, and it ignores the configured operators. Numbering is local to each call, so a caller that joins separately pseudonymized texts must restore each one before joining. See SPEC.md, "Core Interfaces".

### Caller-supplied terms

`recognizers/terms` reports every occurrence of terms the caller knows, such as client names, party names, matter codes or deal code names. It is not in the default pipeline, because it needs the caller's list. Add it with `WithRecognizer`:

```go
r, err := terms.New(recognizers.EntityCustomTerm, "Project Falcon", "Tan Ah Kow", "M-2026-0041")
if err != nil {
    return err // a blank term is rejected here
}
s := piiscrub.NewScrubber(piiscrub.WithRecognizer(r))
out, _ := s.Redact(ctx, "Project Falcon (matter M-2026-0041) for tan ah\nkow")
// [[redactedcustomterm1]] (matter [[redactedcustomterm2]]) for [[redactedcustomterm3]]
```

Matching is case-insensitive, and a run of whitespace in a term matches any run of whitespace in the text. A term must not be glued to a letter or digit, so `Ann` does not match inside `Anna`. Where terms overlap, the longer one wins. `CUSTOM_TERM` matches are numbered per distinct value: `Project Falcon` and `project  falcon` share `[[redactedcustomterm1]]`. Passing `recognizers.EntityPerson` or `recognizers.EntityOrganization` instead reports the terms under that type.

To turn off the default legal and money recognizers, use `WithoutEntityTypes`:

```go
s := piiscrub.NewScrubber(piiscrub.WithoutEntityTypes(
    recognizers.EntityMoney,      // keep amounts in the text
    recognizers.EntityCaseNumber, // keep case numbers in the text
))
```

### Dedicated redaction

`Scrubber.Redact(ctx, text)` and `Anonymizer.Redact(ctx, text, entities)` replace every detected entity with a typed numbered marker such as `[[redactedperson1]]` or `[[redactedsocialnumber2]]`, numbered per type from 1 in the order the entities appear in text. The same type and the same canonical value share one marker, so a repeated value reads as one thing. Every entity type is redacted, even ones with no operator of their own, so nothing is ever left unredacted; on error the result is empty. Unlike `Anonymize`, `Redact` ignores the configured replace/mask/hash operators and always redacts. It is `Anonymizer.Pseudonymize`, or `Scrubber.Pseudonymize` and `Scrubber.PseudonymizeAll`, and it ignores the configured operators. Numbering is local to each call, so a caller that joins separately pseudonymized texts must restore each one before joining. See SPEC.md, "Core Interfaces".

### Options

- `WithMinConfidence(t)` on a `Scrubber` (or `analyzer.WithMinConfidence(t)` on an `Analyzer`) keeps an entity when its confidence is at least `t`. The threshold is inclusive. `NewScrubber` uses 0.70. Raising it above 0.80 filters out the NLP types, which score 0.80.
- `analyzer.WithContextBoost()` raises confidence from surrounding words. It is opt-in, because it changes overlap outcomes. It runs before the threshold and before overlap resolution, and it never lifts a span that overlaps a URL which survives the threshold. `Scrubber` does not enable it; build an `Analyzer` yourself to use it. On the two corpora, exact precision and recall were identical with boosting on and off (SPEC.md, "Core Interfaces" and the corpus measurement).
- `piiscrub.WithoutEntityTypes(types...)` turns entity types off for a `Scrubber`. Their entities are dropped right after the recognizers run, so a disabled type cannot win an overlap and hide another type's span. It is construction-time only; `analyzer.DisableEntityTypes(types...)` is the same switch on an `Analyzer`.
- `anonymizer.WithDefaultOperator(op)` sets the operator for types with no `SetOperator` entry. Without it, an entity whose type has no operator is an error. It is never returned unredacted.

### NLP sidecar (person, organisation, location)

`PERSON`, `ORGANIZATION` and `LOCATION` come from an optional sidecar: a
separate Go module (`sidecar/`) that runs prose NER in its own process. The root
module stays standard-library-only, and the recognizer is opt-in, so a caller
that never starts the sidecar gets exactly the behaviour it had before.

```bash
# build and run the sidecar (127.0.0.1:8787 by default)
cd sidecar && go build ./cmd/piiscrub-sidecar && ./piiscrub-sidecar
```

```go
import "github.com/u007/dataprotect/recognizers/nlp"

s := piiscrub.NewScrubberWithRecognizers(nlp.New("http://127.0.0.1:8787"))
```

To run the combination conservatively — built-in types only, no NLP entities —
raise the threshold above the NLP score:

```go
s := piiscrub.NewScrubber(
    piiscrub.WithRecognizer(nlp.New("http://127.0.0.1:8787")),
    piiscrub.WithMinConfidence(0.85), // drops every NLP entity (0.80)
)
```

`NewScrubber()` reads no environment variable and makes no network call, so the
golden tests stay deterministic. If you want the URL from the environment, read
it yourself (`os.Getenv("PII_SCRUB_NLP_URL")`) or call `nlp.FromEnv()` (it returns an error if
`PII_SCRUB_NLP_BINARY` is set), and pass the recognizer in. `PII_SCRUB_NLP_URL` must be set in the deployment
environment for the sidecar to be used; it is documented in `.env.example`.

To have the library start the sidecar itself, set `PII_SCRUB_NLP_BINARY` to the
`piiscrub-sidecar` binary path (instead of `PII_SCRUB_NLP_URL`) and call
`nlp.ManagedFromEnv(ctx)`, then `Close` the result. Setting both variables is an
error. `nlp.FromEnv()` returns an error when `PII_SCRUB_NLP_BINARY` is set.

`nlp.New("")` is a disabled recognizer: it returns no entities and no error. A
configured but broken sidecar is an error, because `Analyze` aborts on a
recognizer error; `nlp.New(url, nlp.WithNLPBestEffort())` is the opt-in mode
that logs the failure (rate-limited) and keeps the rest of the pipeline running.
NLP entities carry confidence 0.80: strictly below every built-in recognizer
(the lowest is 0.85, the keyword-gated national ID and passport rules) and
above the default 0.70 floor, so they fill gaps in prose rather than overriding
typed entities, and they are not filtered out.

### LLM second pass (MyKad judge)

A bare 12-digit run with no identity-card keyword (for example `ref
900515103421 on the form`) looks like a Malaysian MyKad but is indistinguishable
from an order number or timestamp. The regex recognizer emits such candidates as
weak, below the 0.70 floor, so they are dropped by default. An optional LLM
second pass confirms or rejects each weak candidate against a local
OpenAI-compatible endpoint before the confidence filter.

The pass is opt-in and off by default. Enable it with
`piiscrub.WithLLMJudge(judge)`, where `judge` implements
`analyzer.Judge`. The `recognizers/judge` package provides a client that talks
to `PII_SCRUB_NLP_JUDGE_URL` (loopback only unless
`PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE=1` is set), defaults to the `qwen3-0.6b`
model (Apache-2.0), and caches results in memory. A confirmed candidate is
promoted to keyword-gated confidence (0.85) and reported; a rejected one is
dropped. A judge error aborts `Analyze` — there is no silent fallback.

```go
import "github.com/u007/dataprotect/recognizers/judge"

j, err := judge.New("http://127.0.0.1:8788")
s := piiscrub.NewScrubber(piiscrub.WithLLMJudge(j))
```

Endpoint URL and model name are documented in `.env.example`. See
`DESIGN-llm-second-pass.md` for the design decisions and the model-selection
record.

### What the NLP path adds

The built-in types are pattern and checksum based, so they cannot see a
person's name or a company in running text. That is what the NLP path is for.
These are the actual outputs, measured, not illustrations:

| Input | Without the NLP recognizer | With it |
|---|---|---|
| `John Smith called about the invoice.` | `John Smith called about the invoice.` | `[[redactedperson1]] called about the invoice.` |
| `Acme Corp is based in Frankfurt.` | `Acme Corp is based in Frankfurt.` | `[[redactedorganization1]] is based in [[redactedlocation1]].` |
| `Contact Sarah Jones at Northgate Ventures in Copenhagen.` | `Contact Sarah Jones at Northgate Ventures in Copenhagen.` | `Contact [[redactedperson1]] at [[redactedorganization1]] in [[redactedlocation1]].` |
| `The audit was signed by Beatriz Nunez, the lead auditor, on 4 April.` | unchanged | `The audit was signed by [[redactedperson1]], the lead auditor, on 4 April.` |
| `Our client Halcyon Manufacturing Corp renewed the contract in Munich.` | unchanged | `Our client [[redactedorganization1]] renewed the contract in [[redactedlocation1]].` |
| `Employee 1000000000009 filed the request from the Bangkok office.` | `Employee [[redactedthid1]] filed the request from the Bangkok office.` | `Employee [[redactedthid1]] filed the request from the [[redactedlocation1]] office.` |
| `Email billing@acme-supply.example or call 555-123-4567. Card 4111111111111111.` | `Email <EMAIL> or call ***-***-4567. Card ****-****-****-1111.` | the same — the built-in types already had it |

The last row matters as much as the first. The NLP path **adds** to the
built-in recognizers, it does not replace them: the email, the phone and the
card are still caught exactly as before, and the Thai ID is still typed
`TH_ID`. NLP entities carry confidence 0.80 against every built-in type (the
lowest is 0.85), so in an overlap the built-in type wins and the NLP span fills
the gaps around it.

### Measured NER accuracy

The sidecar's prose backend is measured, not claimed. `make test-ner` starts a
sidecar, scores the detector against `testdata/golden/ner/` (124 cases) and
stops it again:

| Measure | Precision | Recall | What it says |
|---|---|---|---|
| typed | 0.8395 | 0.8718 | the detector calls the right thing the right type |
| span | 0.9012 | 0.9359 | the detector covers the right bytes |

Both are gated. The typed number is the golden package's own, type-sensitive
metric. The span number ignores the type, because all three NER types carry
the same redact operator — a span covered exactly but typed differently is
redacted just the same, so the span number is the one that says the PII is
actually removed.

The generalisation figure is the first evaluation on held-out text the decoder
had never seen: typed P=0.5333 R=0.6154, span P=0.8667 R=1.0000. The gate
bounds sit below the measured baseline, and the honest first-run numbers are
recorded in `ner_accuracy_test.go`.

**On the real corpora the numbers are much worse, and that is the number that
matters.** The corpora label person, organisation and place spans, and
`make corpus` scores them with the same scorer:

| Pipeline (2000 nemotron cases) | exact P | exact R | overlap R |
|---|---|---|---|
| regex + checksum + national + secret | 0.8233 | 0.2654 | 0.2688 |
| with the NLP sidecar | 0.4232 | 0.3903 | 0.4704 |

The regex-only row scores 0.2654 because PERSON, ORGANIZATION and LOCATION
are now in scope and it detects none of them — that is the baseline the NLP
path has to beat, and it beats it on recall at the cost of precision. The
per-type truth with the sidecar is LOCATION P=0.2457 R=0.3556, ORGANIZATION
P=0.3077 R=0.1650, PERSON P=0.1307 R=0.0837.

Two things drive the gap. The corpora annotate a **given name** ("Manal")
while a NER model reports a **full name** ("Manal King") — 74% of the gold
spans are covered, only 35% match exactly. And the corpora are full of form
fields ("First Name:", "Account Number") that the detector can report.

So: **the sidecar is good on running prose and poor on form-filling records.**
Enable it on prose; leave it off for structured forms.

The sidecar is not a general-purpose NER service. It is English-only, and a
brand name with no other signal ("The Orion laptop") is reported as a place.

The NLP path is the slow path, and it is worth knowing the price before you
opt in. On a 239-byte sample, one `Analyze` call takes 305 us with the regex
recognizers alone and 19.6 ms with the sidecar — 64x. About 50 requests a
second sequentially, about 130 a second with 16 concurrent callers. The
regex path is untouched, so a caller that does not enable the NLP recognizer
pays none of this.

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
	// 1,<EMAIL>,card [[redactedcreditcard1]]
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
go test -race ./...      # the same suite under the race detector (make test-race)
go test -v ./recognizers/...
make fmt                 # fails if any file is not gofmt-clean
```

Make targets (from `make help`):

| Target | What it does |
|---|---|
| `build`, `vet`, `test`, `fmt` | build every package, run `go vet`, run the library suite, check gofmt |
| `test-race` | run the library suite under the race detector |
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

Measured with `make benchmark`, `make corpus` and `make presidio`; every number in this section comes from those runs or is copied from [SPEC.md](SPEC.md) and [SOURCES.md](SOURCES.md). The seed set is 655 cases (412 positive, 243 negative), and the committed gate re-measures at exact and overlap P=1.0000 R=1.0000 with tp=449 fp=0 fn=0 and 243 of 243 negative cases clean.

Updated 2026-10-21: the seed set is now 551 cases, because two bare-provider-token cases were added when the glued-prefix and GitHub-body gaps were closed. The committed gate re-measures at exact and overlap P=1.0000 R=0.9974 with tp=383 fp=0 fn=1, and 199 of 199 negative cases clean. The tables below are left as they were measured on 2026-10-19; the corpus rows were re-measured on 2026-10-21 and carry that date.

Updated 2026-10-22: the seed set is now 553 cases, because one positive and one negative were added for the Google Gemini auth key (`AQ.`), the format AI Studio issues instead of `AIza` keys. The gate re-measures at exact and overlap P=1.0000 R=0.9974 with tp=384 fp=0 fn=1, and 200 of 200 negative cases clean. Neither corpus moved: the new shape detects nothing in nemotron or gretel, so it adds no false positives either.

Updated 2026-11-04: the seed set is now 556 cases, because three BITCOIN negatives were added (a bad P2SH checksum, a bad taproot checksum and a mixed-case bech32 address). The gate re-measures at exact and overlap P=1.0000 R=1.0000 with tp=385 fp=0 fn=0, and 203 of 203 negative cases clean; `pos-phone-trunk-zero`, the earlier single miss, is now detected. The corpora were not re-run.

### Sources

Corpora are fetched into `testdata/external/` (gitignored). Set `DATAPROTECT_DATA_DIR` to keep them elsewhere; the fetch script and the Makefile both read it. Each fetched directory holds a `PROVENANCE.txt` with the revision, and each converted directory holds `schema.json`, which records the input's SHA-256 and the row bound.

| Dataset | Source | Pinned revision | Licence | Rows used | Local path | How to fetch |
|---|---|---|---|---|---|---|
| Golden seed set | `testdata/golden/seed.json`, authored in this repository from synthetic text | none; the working tree (see above) | none needed, authored here (DATASET_LICENSES.md, "Provenance") | 655 cases: 412 positive, 243 negative | `testdata/golden/` (committed) | nothing to fetch |
| Nemotron-PII | [nvidia/Nemotron-PII](https://huggingface.co/datasets/nvidia/Nemotron-PII) | `b70ffaf5ff39e079776134c5bf4381f00a9fd1ed` | CC-BY-4.0 (attribution) | 2000 of 100000 rows, test split, file order | `testdata/external/nemotron-pii/` | `scripts/fetch-dataset.sh fetch nemotron-pii`, or `make download` |
| Gretel synthetic PII finance (multilingual) | [gretelai/synthetic_pii_finance_multilingual](https://huggingface.co/datasets/gretelai/synthetic_pii_finance_multilingual) | `7b844d16738527a04264f50214cb426a4cea0897` | Apache-2.0 with NOTICE (attribution) | 2000 of 5596 rows, `combined_test` split only | `testdata/external/gretel-pii-finance/` | `scripts/fetch-dataset.sh fetch gretel-pii-finance`, or `make download` |
| Microsoft Presidio test cases | [microsoft/presidio](https://github.com/microsoft/presidio) | `2523c7b74a469270c5c78bb253f140eafca21e31` | MIT | 592 cases from 601 rows, ten modules; 9 rows set aside | `testdata/external/presidio/` | `make presidio` (clones at the pin and extracts with `scripts/presidio_extract.py`) |

`make download` skips the restricted corpora. For those, see "Datasets considered and not used" below.

### Headline results

| Dataset | Cases | Match | Precision | Recall | F1 | tp / fp / fn |
|---|---|---|---|---|---|---|
| Golden seed | 655 | exact span | 1.0000 | 1.0000 | 1.0000 | 449 / 0 / 0 |
| Golden seed | 655 | byte overlap | 1.0000 | 1.0000 | 1.0000 | 449 / 0 / 0 |
| Nemotron-PII | 2000 | exact span | 0.8841 | 0.3326 | 0.4833 | 3434 / 450 / 6892 |
| Nemotron-PII | 2000 | byte overlap | 0.8921 | 0.3356 | 0.4877 | 3465 / 419 / 6861 |
| Gretel PII finance | 2000 | exact span | 0.4397 | 0.1192 | 0.1875 | 922 / 1175 / 6815 |
| Gretel PII finance | 2000 | byte overlap | 0.4549 | 0.1233 | 0.1940 | 954 / 1143 / 6783 |

Negative cases with no detection: seed 243 of 243; Nemotron-PII 35 of 44; gretel 225 of 272. The corpus rows are measured with the finance and identity labels mapped (SPEC.md, the 2026-11-22 addenda). The table is the regex-only pipeline; the corpus numbers before the finance and identity labels were mapped are in SPEC.md.

The corpus recall fell from the 0.8916 / 0.8201 published before 2026-10-20 because PERSON, ORGANIZATION and LOCATION became in-scope gold on that date (they are detectable only through the NLP sidecar), and this table is the regex-only pipeline, which detects none of them: nemotron carries 3275 PERSON, 1988 LOCATION and 1018 ORGANIZATION false negatives. Scored with the sidecar, the same samples give exact P=0.4376 R=0.4135 — see SPEC.md, "NLP Sidecar".

The seed set is hand-written and small. Its numbers are a floor, not a result. The corpus numbers are real measurements of a 2000-record sample, not a target. The seed set is the only one used as a CI gate.

### Per type

Exact span, per type. A dash means the corpus schema does not label that type. Detections of such a type are reported as out of schema and are not scored: Nemotron-PII has IBAN 1 out of schema; gretel has BITCOIN 21, MY_PASSPORT 1 and URL 817. The seed column gives case counts from `testdata/golden/seed.json` and gold spans from the benchmark.

| Type | Seed cases (gold spans) | Seed P / R (tp / fp / fn) | Nemotron-PII P / R | Gretel P / R |
|---|---|---|---|---|
| `EMAIL` | 51 (57) | 1.0000 / 1.0000 (57 / 0 / 0) | 0.9975 / 0.9975 | 0.3281 / 0.8060 |
| `PHONE` | 54 (58) | 1.0000 / 1.0000 (58 / 0 / 0) | 0.5679 / 0.9982 | 0.2857 / 0.8580 |
| `CREDIT_CARD` | 50 (57) | 1.0000 / 1.0000 (57 / 0 / 0) | 0.9405 / 0.9331 | 0.6875 / 0.7719 |
| `SSN` | 32 (36) | 1.0000 / 1.0000 (36 / 0 / 0) | 0.8780 / 1.0000 | 0.9787 / 0.8846 |
| `IP_ADDRESS` | 44 (46) | 1.0000 / 1.0000 (46 / 0 / 0) | 0.9828 / 0.9771 | 0.8400 / 0.9767 |
| `URL` | 30 (30) | 1.0000 / 1.0000 (30 / 0 / 0) | 0.9900 / 0.9229 | - |
| `IBAN` | 35 (35) | 1.0000 / 1.0000 (35 / 0 / 0) | - | 0.9556 / 0.9773 |
| `BITCOIN` | 13 (13) | 1.0000 / 1.0000 (13 / 0 / 0) | - | - |
| `SECRET` | 37 (37) | 1.0000 / 1.0000 (37 / 0 / 0) | 0.2540 / 0.0804 | 0.5600 / 0.3784 |
| `PERSON` | - | - | 0.0000 / 0.0000 | 0.0000 / 0.0000 |
| `ORGANIZATION` | - | - | 0.0000 / 0.0000 | 0.0000 / 0.0000 |
| `LOCATION` | - | - | 0.0000 / 0.0000 | 0.0000 / 0.0000 |
| `SG_NRIC` | 5 (5) | 1.0000 / 1.0000 (5 / 0 / 0) | - | - |
| `MY_IC` | 3 (3) | 1.0000 / 1.0000 (3 / 0 / 0) | - | - |
| `TH_ID` | 4 (4) | 1.0000 / 1.0000 (4 / 0 / 0) | - | - |
| `MY_PASSPORT` | 3 (3) | 1.0000 / 1.0000 (3 / 0 / 0) | - | - |
| `SG_PASSPORT` | 2 (2) | 1.0000 / 1.0000 (2 / 0 / 0) | - | - |

`PERSON`, `ORGANIZATION` and `LOCATION` are in scope from 2026-10-20 and this table is the regex-only pipeline, which does not implement them: the cells are zero recall against nemotron 3275 / 1018 / 1988 and gretel 2577 / 2374 / 1489 gold spans. With the NLP sidecar they become nonzero but stay weak (SPEC.md, "NLP Sidecar").

Upper bounds. The benchmark also reports precision with the set-aside detections left out. Nemotron-PII exact precision is 0.9808 (470 detections set aside) and overlap precision 0.9922. Gretel exact precision is 0.3843 (13 set aside) and overlap precision 0.4014. An upper bound is not accuracy. It assumes that every set-aside detection is correct, and the set-aside spans are the ones the corpus labels with a name we do not map to a type, so some of them could be real errors. SPEC.md says the Nemotron-PII PHONE bound should not be quoted as meaningful: most of its set-aside detections sit on ID-type fields that the bare 10-digit phone branch matches. They are PII under the corpus labels and the scrubber redacts them, so this is a label-scope artefact, not a detector error.

### Why gretel precision is low

Most of the gretel false positives are two types: EMAIL (383) and PHONE (695), out of 1128. For EMAIL, SPEC.md attributes the gap to the corpus. Some records label only the local part as `user_name`. A Markdown `mailto:` link renders the address twice while the corpus labels it once. In records that carry any email label, only 106 of 647 email-shaped strings are labelled `email`. For PHONE, almost all of the 695 gretel false positives sit on digit runs the corpus leaves unlabelled: EDIFACT, SWIFT, XML and CSV values, placeholders such as `1234567890`, and ID fields. The bare 10-digit branch of the phone pattern matches them. About 40 are real-looking phone numbers the corpus missed, so the true precision is higher than measured. No narrowing rule was adopted, because each candidate either drops a gold phone or stops redacting numbers that could be phones (TODO.md, 2026-10-19). The EMAIL attribution is also still open: TODO.md keeps "Corpus-side label quality depresses measured precision" unchecked.

### Presidio test cases

The Presidio measurement covers the ten modules whose entity type this library implements. Each module is scored as its own dataset, so the schema is that module's type. Exact span counts are `(tp, fp, fn)`. The overlap column counts a span that touches a gold span as a match, so it is an upper bound.

| Module | Type | Cases | Negatives | Exact P / R (tp, fp, fn) | Overlap P / R |
|---|---|---|---|---|---|
| `email` | EMAIL | 8 | 2 | 1.0000 / 1.0000 (7, 0, 0) | 1.0000 / 1.0000 |
| `ip` | IP_ADDRESS | 114 | 29 | 0.8095 / 0.7556 (68, 16, 22) | 1.0000 / 0.9333 |
| `url` | URL | 16 | 3 | 0.7500 / 0.6429 (9, 3, 5) | 0.9167 / 0.7857 |
| `iban` | IBAN | 329 | 172 | 1.0000 / 1.0000 (158, 0, 0) | 1.0000 / 1.0000 |
| `crypto` | BITCOIN | 9 | 3 | 1.0000 / 1.0000 (7, 0, 0) | 1.0000 / 1.0000 |
| `credit_card` | CREDIT_CARD | 18 | 5 | 0.8667 / 0.8667 (13, 2, 2) | 0.8667 / 0.8667 |
| `phone` | PHONE | 21 | 0 | 0.7500 / 0.7241 (21, 7, 8) | 0.9286 / 0.8966 |
| `us_ssn` | SSN | 28 | 12 | 0.5769 / 0.8824 (15, 11, 2) | 0.5769 / 0.8824 |
| `th_tnin` | TH_ID | 39 | 26 | 0.8125 / 1.0000 (13, 3, 0) | 0.8125 / 1.0000 |
| `sg_fin` | SG_NRIC | 10 | 1 | 1.0000 / 0.5000 (5, 0, 5) | 1.0000 / 0.5000 |

Totals: 592 cases from 601 rows. 9 rows are set aside: 2 exact-duplicate texts (the `5555555555554444` card repeated in two lists, and the bare `2400:c401::5054:ff:fe1b:b031` repeated in a second context — each counted once), 4 leniency-variant duplicates of texts the default recognizer already covers, 1 count-only row with no determinable span, and 2 empty-text negatives (SPEC.md, "Presidio test cases"). For `ip`, SPEC.md explains the exact-span misses as span conventions: CIDR suffixes, zone IDs, and the unspecified `::`, which is not redacted by design. Phone national formats are out of scope. Neither SPEC.md nor TODO.md analyses the `url` misses. The five `sg_fin` misses were checked by running the recognizer on each value: `M4332674T` has an unsupported M prefix; `A1234567Z` and `B1234567Z` have prefixes outside S, T, F and G; and `T1234567Z` and `G1122144L` fail the recognizer's check letter. Presidio's gold labels all five.

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

The secret recognizer has no external corpus. Its only scored data is the 37 SECRET cases in the seed set. The corpora annotate some secrets under labels outside the type map (gretel `api_key`; nemotron `api_key` and `http_cookie`). Those spans are unlabelled, not scored, so their SECRET detections are out of schema and not scored. Nothing from the Kaggle competition is used yet.

## Known limits

Each item links to its open entry in [TODO.md](TODO.md) or to the rule in [SPEC.md](SPEC.md).

- **SSN is shape-only.** `666-12-3456` is reported, because Presidio's never-issued rules are not applied (TODO.md, "SSN: shape-only").
- **Phone national formats.** Israel, Japan and UK national formats are not matched (TODO.md, "Phone: national formats").
- **Truncated international numbers.** `+55 11 98456 5666` is reported as `+55 11 98456`, so `5666` stays visible. `+33 1 42 68 53 00` is reported as `+33 1 42 68`. This was accepted as a user decision on 2026-10-16 (TODO.md, "Phone: truncated international numbers").
- **Bare domains.** `microsoft.com` is not detected (TODO.md, "URL: bare domains").
- **Merged card spans.** A span merged by the card-window search can cover two cards, or a card and a year. Its one placeholder restores as one original (TODO.md, "Merged card spans are one placeholder").
- **Secret over-redaction.** A plain word of eight or more characters after a label is reported: `Password: Required` anonymizes to `Password: [[redactedsecret1]]`. Uniform values such as `00000000000000000000` are not reported. Dotted tails are kept or trimmed by a fixed rule (SPEC.md, "Recognizers", secret row).
- **Label boundary, accepted cost.** `pass` and `pwd` need a non-letter before them, so `bypass=` and `compass:` are not reported. The same rule means `dbpass=x` and `rootpwd=x` are not reported either (TODO.md, secret section, 2026-10-19).
- **Closing bracket after a password.** One trailing `)`, `]` or `}` is dropped when whitespace or the end of text follows it and the value has no opener for it, so `(pass=Hunter2xyz)` reports `Hunter2xyz`. A closing bracket followed by a quote (`"Hunter2xyz)"`) is kept and over-redacts one byte; it does not leak the value (TODO.md, secret section, 2026-10-19).
- **Provider tokens with no label, limits.** The formats in the SECRET row are matched by fixed prefix and shape only, so a token that does not follow the exact length is not reported (a GitHub token one character short is missed). Not covered: Slack `xoxa-`, `xoxr-` and `xoxs-` (no provider source found; see TODO.md), bare JWE, a bare JWT with a fourth segment, Stripe test keys (`sk_test_`), Stripe `sk_org_`, and the formats not listed. A token inside a URL is redacted as part of the URL, since the URL recognizer scores higher (0.99 against 0.98). Stated limits of the source-backed formats are in SOURCES.md.
- **Other open items.** Singapore M-prefix NRIC is not supported. Thai passports are not covered. BITCOIN is not labelled by either corpus, so it is unmeasured there. On Nemotron-PII, all 230 CREDIT_CARD false negatives fail the Luhn check and carry a valid network prefix; the label-style keyword gate added 2026-10-21 recovers most of them (nemotron P/R 0.6154/0.0945 → 0.9405/0.9331; gretel 0.5745/0.4737 → 0.6875/0.7719) while keeping the seed negatives clean, and it adds no false positives on either corpus — every remaining one was already present under Luhn-only. The phone-vs-card collision those gates exposed is closed by the same date: CREDIT_CARD now outranks PHONE in overlap resolution whatever their scores, so a gated card is masked as a card instead of as the phone number covering its tail (SPEC.md, "Corpus measurement").

## License

piiscrub is released under the MIT licence; see [LICENSE](LICENSE).

The corpora named above are not covered by that licence and are not redistributed here. Each carries its own terms, recorded in [DATASET_LICENSES.md](DATASET_LICENSES.md). Nemotron-PII (CC-BY-4.0) and the gretel corpus (Apache-2.0 with NOTICE) require attribution. When you publish a result, cite the dataset and its revision, as the tables above do. Do not copy corpus content into `testdata/golden/`.

### Optional components

Some features live behind optional components. Each component carries its own
licence, and you inherit that component's terms only when you choose to use it:

| Component | Licence (SPDX) | Trigger | You are bound if you… |
|-----------|---------------|---------|----------------------|
| piiscrub core | MIT | any import of `github.com/u007/dataprotect` | use any part of the library |
| `onnxruntime_go` | Apache-2.0 *(unverified — module download blocked)* | `NER_BACKEND=onnx` sidecar build (`make build-sidecar-onnx`) | compile/run the sidecar with `-tags onnx` |
| `go-face` | CC0-1.0 | `NewDetector()` when `libdlib-dev` is present | call face detection |
| dlib library | Boost-1.0 | `libdlib-dev` installed | compile the sidecar via cgo |
| `shape_predictor_5_face_landmarks.dat` | verify (dlib.net model) | `RealDetector.DetectFaces()` | download the model on first face detection |
| `dlib_face_recognition_resnet_model_v1.dat` | verify (dlib.net model) | `RealDetector.DetectFaces()` | download the model on first face detection |
| Benchmark datasets | per [DATASET_LICENSES.md](DATASET_LICENSES.md) | `make corpus` / `make corpus-nlp` | score or convert any corpus |

If you do not opt into a component, its licence does not affect your use of
piiscrub. The root module stays standard-library-only; `go-face` and
`onnxruntime_go` live only in the separate `sidecar/` module.

For full third-party terms with source URLs and obligations, see
[THIRD_PARTY.md](THIRD_PARTY.md).

*Not legal advice: this table documents upstream licences as recorded at the
time of writing. Licences can change. Some corpus terms (notably ai4privacy's
staff-count restriction) may restrict commercial use of anything trained or
evaluated on them. For a commercial launch, have counsel confirm.*

## More

- [SPEC.md](SPEC.md): architecture, recognizer rules, corpus measurement and Presidio measurement.
- [SOURCES.md](SOURCES.md): where each dataset, rule and reference came from, and when it was checked.
- [TODO.md](TODO.md): open items and their decisions.
- [AGENTS.md](AGENTS.md): development conventions, including threshold semantics and the module split.
- [DATASET_LICENSES.md](DATASET_LICENSES.md): dataset terms and how they relate to this repository's licence.
