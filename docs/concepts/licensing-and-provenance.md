---
type: Concept
title: Licensing & Provenance — Opt-In Disclosure Pattern
description: 'How piiscrub discloses licences: core MIT plus optional components that bind you only when opted into. Records the disclosure table structure, where each licence document lives (README.md, THIRD_PARTY.md, DATASET_LICENSES.md, SOURCES.md), what is verified versus unverified (onnxruntime_go and the two dlib face models), and the not-legal-advice caveat.'
resource: 'THIRD_PARTY.md; README.md ## License; DATASET_LICENSES.md; SOURCES.md'
tags:
  - licensing
  - third-party
  - provenance
  - compliance
  - notice
  - readme
timestamp: 2026-11-12T02:54:58Z
---
# Licensing & Provenance — Opt-In Disclosure Pattern

piiscrub's licensing story is deliberately split: a permissive core, plus
optional components whose terms attach **only** when you choose to use them.
This document records that pattern, the disclosure table shape, where each
licence document lives, and what is verified versus still unverified. It does
not restate the tables themselves — those live in the repo-root files named
below and are the source of truth.

## The opt-in principle

- **Core is MIT.** `LICENSE` at the repo root is the MIT licence
  (Copyright (c) 2026 Mercstudio). Any import of
  `github.com/u007/dataprotect` binds you to MIT and nothing else.
- **Optional components inherit their own terms only when opted into.** The
  root module stays standard-library-only. `go-face`, `onnxruntime_go` and the
  dlib cgo dependency live only in the separate `sidecar/` module and are
  activated by an explicit build tag or runtime switch. If you do not opt in,
  their licences do not affect your use of piiscrub.
- **Corpora are never bundled.** No third-party dataset is committed to the
  tree; `testdata/benchmarks/`, `testdata/external/` and `datasets/` are
  gitignored. This is what stops a dataset's terms from reaching the source
  code. Corpora are fetched by `scripts/fetch-dataset.sh` and scored locally.

The stated rationale: one table cannot honestly describe a library whose
face-detection path pulls in dlib and whose ONNX path pulls in onnxruntime, so
each component gets a row and a trigger.

## Disclosure table structure

Two tables share the same shape, one at each audience level:

- **README.md → `## License` → `### Optional components`** (reader-facing,
  short). Columns:
  `Component | Licence (SPDX) | Trigger | You are bound if you…`
- **THIRD_PARTY.md** (full inventory, repo root). Columns:
  `Component | SPDX | Source | Trigger | Obligations` — adds upstream source
  URLs/pinned versions and the concrete obligation (e.g. "keep copyright
  notice", "none (public domain dedication)", "verify per dlib.net terms
  before download").

Both tables follow the same rules:

- The **Trigger** column states the exact build flag, env var, runtime call or
  command that pulls the component in (e.g. `NER_BACKEND=onnx` sidecar build,
  `NewDetector()` when `libdlib-dev` is present, `RealDetector.DetectFaces()`,
  `make corpus`).
- **Unverified rows are marked inline** (`*(unverified — module download
  blocked)*`, `verify (dlib.net model)`) rather than being omitted or asserted.
- The README table cross-references THIRD_PARTY.md for full terms; THIRD_PARTY
  cross-references DATASET_LICENSES.md for corpora.

## Where each document lives

| File | Scope | Maintained as |
|------|-------|---------------|
| `LICENSE` | Core library licence (MIT) | static |
| `README.md` `## License` | Short optional-component table + opt-in statement + not-legal-advice caveat | reader-facing summary |
| `THIRD_PARTY.md` | Full third-party licence inventory for code/deps/model weights | authoritative inventory |
| `DATASET_LICENSES.md` | Corpus terms, commercial-use restrictions, obligations | dataset terms, dated review |
| `SOURCES.md` | Provenance and refresh: where each dataset/rule came from, checked-when | provenance, not licences |

`.env.example` carries runtime pointer comments for `NER_BACKEND`,
`NER_ONNX_MODEL` and `NER_ONNX_RUNTIME_LIB` that name THIRD_PARTY.md, so a user
enabling the ONNX backend sees the licence pointer at the point of opt-in.

## Verified versus unverified

Verified by reading the actual licence text / copyright file:

- **piiscrub core — MIT.** Read from `LICENSE`.
- **go-face — CC0-1.0.** Read from the module cache `COPYING` file.
- **dlib library — Boost-1.0.** Read from
  `/usr/share/doc/libdlib-dev/copyright`.
- **`dlib-models/shape_predictor_68_face_landmarks.dat` — CC0-1.0.** Read from
  the system copyright file. **This is NOT the model piiscrub uses** —
  `RealDetector` downloads the 5-point landmark and ResNet recognition models
  instead. Do not let this row imply the used models are CC0.

Unverified (recorded as `verify`, not asserted):

- **`onnxruntime_go` — Apache-2.0 claimed but unverified.** Module download was
  blocked, so the declared licence in `sidecar/go.mod` (`v1.35.0`) has not been
  checked against the upstream licence file.
- **`shape_predictor_5_face_landmarks.dat` and
  `dlib_face_recognition_resnet_model_v1.dat` — licence unverified.** They are
  downloaded on first use from dlib.net and are not bundled. Model-weight
  licences can differ from the dlib library's Boost-1.0, and some dlib models
  trained on iBUG 300-W carry non-commercial restrictions. Verify each model
  file at dlib.net before download.

## Not-legal-advice caveat

Every licence surface (README table, THIRD_PARTY.md, DATASET_LICENSES.md) ends
with the same disclaimer in substance: the table documents upstream licences as
recorded at the time of writing; licences can change; some corpus terms (notably
ai4privacy's staff-count restriction) may restrict commercial use of anything
trained or evaluated on them; for a commercial launch, have counsel confirm.
The dataset table adds "verify before use" and a dated review stamp.

## Status (2026-11-12)

The README `## License` expansion, the repo-root `THIRD_PARTY.md`, and the
`.env.example` pointer comments were authored together on 2026-11-10.
`THIRD_PARTY.md` was **uncommitted** when this note was first written
(`git status` showed `?? THIRD_PARTY.md`), but it is now **committed in
`3bf03f2`** (docs(licensing): add optional-components disclosure and
THIRD_PARTY inventory), together with the README and `.env.example` changes, so
the inventory in the tree is the published record. Treat the files, not this
note, as the authoritative licence record, and re-verify the two model weights
and `onnxruntime_go` before relying on their SPDX values.