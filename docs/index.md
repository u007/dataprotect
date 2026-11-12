---
okf_version: 0.1
---

# concepts

- [Licensing & Provenance — Opt-In Disclosure Pattern](concepts/licensing-and-provenance.md) - How piiscrub discloses licences: core MIT plus optional components that bind you only when opted into. Records the disclosure table structure, where each licence document lives (README.md, THIRD_PARTY.md, DATASET_LICENSES.md, SOURCES.md), what is verified versus unverified (onnxruntime_go and the two dlib face models), and the not-legal-advice caveat.
- [piiscrub — Development Conventions & Confidence Semantics](concepts/development-conventions.md) - Contributor and agent conventions for piiscrub: build and test commands, package map, confidence and threshold semantics (inclusive >=, 1e-9 quantisation via roundConfidence, and why Go constant arithmetic makes float-precision tests silently vacuous), overlap tie-breaking policy, Go 1.24 idioms, and the rule against real PII in test fixtures.
- [piiscrub — Usage & Quick Start](concepts/piiscrub-usage.md) - Entry point for using the piiscrub Go library: feature list (8 regex, 2 checksum, 7 national-ID and passport recognizers, 1 secret recognizer, 4 operators), installation, two quick-start paths (manual Analyzer plus Anonymizer wiring, and the high-level Scrubber), measured accuracy over the 556-case seed and its per-entity-type coverage gaps, the fetchable external corpora and their licences, the on-demand corpus measurement (`make corpus`) with its real numbers, and pointers to the architecture spec and development conventions.

# decisions

- [LLM second pass — model choice (Qwen3-0.6B, Apache-2.0)](decisions/llm-second-pass-model.md) - Resolves decision 7 of the optional LLM second-pass design: Qwen3-0.6B (Apache-2.0) is the judge model, with licence verification and the rejected alternatives. Phases 1 (weak-candidate marker) and 2 (judge client in recognizers/judge) are implemented; Phase 3 has an unwired 10-case MyKad fixture.
- [NLP managed mode, ONNX and LLM second pass — session findings (2026-11-12)](decisions/2026-11-12-session-findings.md) - Dated 2026-11-12 record: managed-NLP Sonnet review items resolved/open, ONNX success-path tests and the Phase 1 licence block, LLM second-pass Phase 3 fixture, the France phone gap, the licensing status correction, and a same-session follow-up that closes item (3) and partly closes (6).
- [piiscrub v1 Spec — Scope, Architecture & Non-Goals](decisions/spec-v1.md) - Architecture decision record for piiscrub v1: why a dependency-free Go PII library instead of Python Presidio, the explicit non-goals, MVP scope versus v2+ phases, core interfaces (Recognizer, Operator, Analyzer, Anonymizer), the actual on-disk package layout, per-recognizer accuracy targets alongside measured results from the 553-case golden seed AND from a 2000-record sample of both fetched corpora (which fall well short of the >99% target, with every large gap explained), operator semantics, the CSV batch contract, pinned dependencies, and the testing/benchmark strategy.

# superpowers

- [Corpus converter and corpus scoring mode — design](superpowers/specs/2026-10-08-corpus-converter-design.md) - Design for tools/corpusbench Parquet→golden conversion and in-schema corpus scoring; implemented
- [Credit-card keyword-gated rule — design](superpowers/specs/2026-10-20-credit-card-keyword-gate-design.md) - Hybrid keyword gate for Luhn-valid card numbers; implemented 2026-10-21 in recognizers/checksum
- [Multilingual NER backend — design](superpowers/specs/2026-10-28-multilingual-ner-design.md) - ONNX multilingual NER backend for the sidecar; Phase 0 landed 2026-11-01, success-path session tests added 2026-11-12, Phase 1 still blocked on a licence-compliant six-language Apache-2.0/MIT checkpoint.

