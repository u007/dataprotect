---
okf_version: 0.1
---

# concepts

- [piiscrub — Development Conventions & Confidence Semantics](concepts/development-conventions.md) - Contributor and agent conventions for piiscrub: build and test commands, package map, confidence and threshold semantics (inclusive >=, 1e-9 quantisation via roundConfidence, and why Go constant arithmetic makes float-precision tests silently vacuous), overlap tie-breaking policy, Go 1.24 idioms, and the rule against real PII in test fixtures.
- [piiscrub — Usage & Quick Start](concepts/piiscrub-usage.md) - Entry point for using the piiscrub Go library: feature list (8 regex and 2 checksum recognizers, 4 operators), installation, two quick-start paths (manual Analyzer plus Anonymizer wiring, and the high-level Scrubber), and pointers to the architecture spec and development conventions.

# decisions

- [piiscrub v1 Spec — Scope, Architecture & Non-Goals](decisions/spec-v1.md) - Architecture decision record for piiscrub v1: why a dependency-free Go PII library instead of Python Presidio, the explicit non-goals, MVP scope versus v2+ phases, core interfaces (Recognizer, Operator, Analyzer, Anonymizer), the actual on-disk package layout, per-recognizer accuracy targets alongside measured results from the 18-case golden seed, operator semantics, the CSV batch contract, pinned dependencies, and the testing/benchmark strategy.

