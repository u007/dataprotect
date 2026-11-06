# Autonomous decisions log (2026-11-04)

Made without user input, per instruction. Review and override as needed.
Source of pending items: unchecked entries in `TODO.md`.

| # | TODO item | Options | Decision | Why |
|---|-----------|---------|----------|-----|
| 1 | SG NRIC M-prefix FIN | (a) enable the third-party check table; (b) keep unsupported until an official ICA source is found | **(b) keep unsupported** | Only third-party sources; the table was already reverted once. A wrong table gives false positives and false negatives. No code change. |
| 2 | Gretel CREDIT_CARD spans narrower than digit run | (a) change recognizer to match corpus; (b) leave, corpus-side issue | **(b) leave** | The TODO says no recognizer change. Matching a corpus labelling quirk would hurt real use. |
| 3 | Multilingual NLP model / ONNX checkpoint | (a) adopt Davlan (AFL-3.0); (b) Babelscape (CC BY-NC-SA); (c) unlicensed MIT re-uploads; (d) defer until an Apache-2.0/MIT checkpoint with the six languages exists | **(d) defer** | The design bar is Apache-2.0/MIT plus six languages, and no surveyed checkpoint meets it. Adopting one would be a licensing decision for the owner. |
| 4 | ONNX success-path tests (Phase 1) | (a) write tests that skip without a model; (b) wait for a real model dir | **(b) wait** | Needs `libonnxruntime` and a model. Skipped tests are forbidden by the global rules. Blocked on #3. |
| 5 | `<s>`/`</s>` special tokens per chunk | (a) assume required; (b) leave unverified | **(b) leave unverified** | Needs the real checkpoint to verify. Blocked on #3. |
| 6 | libtokenizers pins for other platforms | (a) pin unverified hashes; (b) defer | **(b) defer** | Downloads are blocked here, and a SHA-256 can't be made up. Needs a machine for each platform. |
| 7 | Opset and quantisation logging | (a) parse the protobuf; (b) defer | **(b) defer** | Low value and adds a dependency. Revisit with the real model. |
| 8 | Managed process mode (Phase F) | do now / defer | **defer** | Marked optional and future in the plan. |
| 9 | Image PII (go-face) | do now / defer | **defer** | Large and out of the current scope. It needs a native dependency and a dataset. |
| 10 | Benchmark suite (OpenNER, PII-Bench, etc.) | do now / defer | **defer** | Needs dataset downloads, which are blocked here. |
| 11 | National ID gaps (MyKad POB codes, Thai bare 13-digit) | (a) add unconfirmed MyKad codes; (b) leave | **(b) leave** | The code list is unconfirmed. The Thai form was already decided on 2026-10-10. |
| 12 | BITCOIN coverage | add synthetic golden cases / leave | **delegated to a Haiku agent** (see result below) | It is the only item that can be done offline. |

## Result of #12 (BITCOIN)
The golden set already had 14 positives and 4 negatives. The Haiku agent added 3 negatives to `testdata/golden/seed.json`: a bad P2SH checksum, a bad taproot checksum, and mixed-case bech32. It raised `minCases` from 553 to 556 in `golden/accuracy_test.go`. I re-ran `go test ./golden/` and it passes. The agent also reported `go test ./...` green. I did not re-run that.
Not updated: docs that quote the case count. The "551 cases" comment in `accuracy_test.go` is also unchanged. Nothing is committed.

## Revisions after advisor review
- **Downloads work.** `curl` to GitHub and Hugging Face returned 200, so the "blocked" premise behind #3, #6 and #10 was wrong.
- **#6 libtokenizers pins: done.** The Makefile now picks the asset and SHA-256 by host (linux x86_64/arm64, darwin arm64/x86_64). I measured the hashes from the v1.27.0 assets, and the linux x86_64 value matched the old pin. `make fetch-libtokenizers` passes on this host; the other three platforms are untested (noted in `TODO.md`).
- **#3 and #10 stay deferred.** #3 is blocked by the licence bar, which is your call, not by downloads. #10 is a large effort I didn't start.
- **Second gate counter found.** The recall bounds in `golden/accuracy_test.go` were 384/385, but the measured value is now 385/385 (`pos-phone-trunk-zero` is detected). I ratcheted them to 1.0, fixed the stale "551 cases" comment and added a Batch 24 note. `go test ./...` passes in the root and in `sidecar/` (run separately; `go.work` covers both).
- **TODO.md:** updated the libtokenizers, BITCOIN and "Multilingual NLP model" items, and added a docs-refresh item.
- **Docs refreshed:** README.md, SPEC.md and AGENTS.md aggregate counts, the `scripts/docs_check.py` constants, and the `docs/` bundle (by `okf:context-writer`, then `make docs-sync`). `python3 scripts/docs_check.py` is in sync. SPEC.md's per-recognizer table was re-measured with `make benchmark` on 2026-11-05: PHONE and CREDIT_CARD are now 1.0000/1.0000, so I removed that `TODO.md` item.
- **Nothing committed.**

## Not done and why
Almost every open item is blocked on external things: model licensing, downloads, per-platform assets or official specs. I did not work around these, per the no-workaround rule. No `TODO.md` entries were changed.

## Notes
- Opus advisor: the first pass had no advisor tool. A later call reviewed this log. It challenged the "downloads blocked" premise and asked for several follow-ups (see "Revisions after advisor review").
- `okf:context` was dispatched at the start. Its digest agrees with decisions 3 to 7: Phase 1 is blocked on a licence-compliant checkpoint, and `test-ner-onnx` doesn't exist yet.
- Stale docs found by the digest. I did not edit them: `docs/` is off-limits to me, and I left `TODO.md` unchanged.
  - The design doc `docs/superpowers/specs/2026-10-28-multilingual-ner-design.md` §12 ends with "the compile gate has not been run". §8 and the 2026-11-01 log say `make onnx-check` is green.
  - The design doc §8 lists `make test-ner-onnx`, which isn't in the Makefile yet (Phase 1).
  - `TODO.md` line 223 ("Multilingual NLP model", deferred because downloads are blocked) overlaps the ONNX track and is probably stale.
  - Suggested follow-up: have `okf:context-writer` fix the two design-doc lines, and fold the `TODO.md` line 223 item into the ONNX items.

## 2026-11-05: MyKad and LLM second pass
- Keyword-free bare 12-digit MyKad stays undetected by default: 20 of 229 bare 12-digit runs in the corpora pass the date check (about 9%).
- Allowed filler words (`is`, `number`, `no`) between the keyword and the digits, so `my ic is 801214105423` is detected. Up to two, and the two must differ: `mykad no no <digits>` is not matched.
- LLM second pass: designed, not built. You asked for it as an optional flag. See `DESIGN-llm-second-pass.md` for the options and my choices.
