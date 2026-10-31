---
okf_version: 0.1
---

# concepts

- [piiscrub — Development Conventions & Confidence Semantics](concepts/development-conventions.md) - Contributor and agent conventions for piiscrub: build and test commands, package map, confidence and threshold semantics (inclusive >=, 1e-9 quantisation via roundConfidence, and why Go constant arithmetic makes float-precision tests silently vacuous), overlap tie-breaking policy, Go 1.24 idioms, and the rule against real PII in test fixtures.
- [piiscrub — Usage & Quick Start](concepts/piiscrub-usage.md) - Entry point for using the piiscrub Go library: feature list (8 regex, 2 checksum, 5 national-ID and passport recognizers, 1 secret recognizer, 4 operators), installation, two quick-start paths (manual Analyzer plus Anonymizer wiring, and the high-level Scrubber), measured accuracy over the 553-case seed and its per-entity-type coverage gaps, the fetchable external corpora and their licences, the on-demand corpus measurement (`make corpus`) with its real numbers, and pointers to the architecture spec and development conventions.

# decisions

- [piiscrub v1 Spec — Scope, Architecture & Non-Goals](decisions/spec-v1.md) - Architecture decision record for piiscrub v1: why a dependency-free Go PII library instead of Python Presidio, the explicit non-goals, MVP scope versus v2+ phases, core interfaces (Recognizer, Operator, Analyzer, Anonymizer), the actual on-disk package layout, per-recognizer accuracy targets alongside measured results from the 549-case golden seed AND from a 2000-record sample of both fetched corpora (which fall well short of the >99% target, with every large gap explained), operator semantics, the CSV batch contract, pinned dependencies, and the testing/benchmark strategy.
