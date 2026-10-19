# Directory Update Log





## 2026-10-19

* **Update**: piiscrub README rewritten — setup and examples compile-checked, results re-measured (`make benchmark`, `make corpus`, `make presidio`), datasets used and not used, known limits; TODO gains an open secret item (a password value keeps a trailing `)`) ([README](../README.md))
* **Update**: piiscrub v1 Spec and README — secret short labels `pass` and `pwd` need a non-letter before them (`bypass=`, `compass:` no longer reported; `dbpass=` and `rootpwd=` no longer matched, accepted); golden seed 534 to 536 cases ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub v1 Spec and README — secret password drops a sentence-final period, a trailing dotted token segment is kept at end of text and trimmed before whitespace, four-segment JWT captured whole; golden seed 530 to 534 cases ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub v1 Spec — secret JWT header under 32 characters and five-segment JWE matched, dot not counted as a character class ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub v1 Spec and README — secret character-class rule removed (uniform values only), JSON-quoted and spaced labels, dotted tokens of any segment count, `dir` JWE with an empty segment; golden seed 518 to 530 cases ([decisions/spec-v1.md](/decisions/spec-v1.md))

## 2026-10-16

* **Update**: piiscrub v1 Spec and README — golden seed expanded from 81 to 491 hand-labelled cases; measured table and known misses ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub — AGENTS.md overlap rules: context boost never lifts a span over a surviving URL ([concepts/development-conventions.md](/concepts/development-conventions.md))
* **Update**: piiscrub v1 Spec — context boost over URLs, PseudonymRoundTrip alignment check, edge limitation ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub v1 Spec — email punycode TLD, phone parenthesised area code, phone numbers after the change ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub — email Unicode IDN addresses, IPv6 label residuals kept ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub — tail-aligned mask for phone and card, dash-separated IBAN, card after a dash IBAN ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub v1 Spec — canonical pseudonym keys, PseudonymizeAll, IBAN country lengths, all-zero card ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub v1 Spec — placeholder collision checked for texts with no entities ([decisions/spec-v1.md](/decisions/spec-v1.md))

## 2026-10-15

* **Update**: piiscrub v1 Spec — IPv6 label fix, linear IP scan and containment check ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub v1 Spec — scope decisions, IPv6 detection ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub v1 Spec — spaced IBAN cut from the right, dash- or dot-joined card ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub — Usage & Quick Start, Pseudonymize example ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))
* **Update**: piiscrub v1 Spec — `Pseudonymize` contract ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub v1 Spec — Scope, Architecture & Non-Goals ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub — Usage & Quick Start ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))
* **Update**: piiscrub — Development Conventions & Confidence Semantics ([concepts/development-conventions.md](/concepts/development-conventions.md))

## 2026-10-14

* **Update**: piiscrub v1 Spec — Scope, Architecture & Non-Goals ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub — Usage & Quick Start ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))
* **Update**: piiscrub — Development Conventions & Confidence Semantics ([concepts/development-conventions.md](/concepts/development-conventions.md))

## 2026-10-08

* **Update**: piiscrub v1 Spec — Scope, Architecture & Non-Goals ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub — Usage & Quick Start ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))

## 2026-10-07

* **Update**: piiscrub v1 Spec — Scope, Architecture & Non-Goals ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub — Usage & Quick Start ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))
## 2026-10-06

* **Update**: piiscrub — Usage & Quick Start ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))
* **Update**: piiscrub — Usage & Quick Start ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))
* **Update**: piiscrub — Usage & Quick Start ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))
## 2026-10-05

* **Update**: piiscrub — Usage & Quick Start ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))
* **Update**: piiscrub v1 Spec — Scope, Architecture & Non-Goals ([decisions/spec-v1.md](/decisions/spec-v1.md))
* **Update**: piiscrub — Development Conventions & Confidence Semantics ([concepts/development-conventions.md](/concepts/development-conventions.md))
* **Update**: piiscrub — Usage & Quick Start ([concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md))
* **Update**: piiscrub v1 Spec — Scope, Architecture & Non-Goals ([decisions/spec-v1.md](/decisions/spec-v1.md))