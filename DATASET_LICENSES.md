# Dataset Licences

No third-party corpus is committed to this repository. Every dataset below is
fetched by a script under `scripts/` and scored locally; the `.gitignore`
excludes `testdata/benchmarks/`, `testdata/external/` and `datasets/`. Keeping
the data out of the tree is deliberate: it is what stops a dataset's terms from
reaching this source code.

**Verify before use.** This table is a starting point recorded on 2026-10-05,
not legal advice, and several entries are ambiguous or need re-checking. A
licence can change; a dataset can be relicensed or withdrawn. Re-read the primary
source before relying on any row.

## Terms summary

| Dataset | Licence | Commercial use | Obligations | Notes |
|---------|---------|----------------|-------------|-------|
| `testdata/golden/seed.json` (this repo) | none needed — authored here | yes | none | Synthetic text written for this repo. No corpus-derived content. See "Provenance" below. |
| OpenNER 1.0 | CC-BY 4.0 (the compilation) | yes | attribution | **Individual constituent corpora keep their own licences, some of which restrict commercial use.** The compilation licence does not override them. |
| PIIBench (`Pritesh-2711/pii-bench`) | apache-2.0 **declared**, but explicitly "derived from multiple sources; users must comply with the original dataset licenses" | depends on constituents | inherits constituents' terms | **Treat the apache-2.0 tag as unreliable.** It consolidates ten datasets including CoNLL-2003 (Reuters-derived) and ai4privacy (custom corporate licence), so the effective terms are the intersection of all ten. |
| ai4privacy/pii-masking-43k | **Custom dual-tier, NOT CC-BY-SA** | yes, up to 3 staff / non-profit / evaluation | attribution, no re-licensing of derivatives | **Corrects an earlier claim in this repo that it was CC-BY-SA.** Free only for individuals, for-profits with ≤3 staff, non-profits, or evaluation. Larger profit-oriented organisations must buy a corporate licence. Derivatives may not be sold or sublicensed. |
| ai4privacy/pii-masking-openpii-1m (current flagship) | `license: other`, declared `cc-by-4.0` | yes | attribution | Licence tag and declared name disagree; read the `LICENCE` file before use. |
| nvidia/Nemotron-PII | CC-BY 4.0 | yes | attribution | Synthetic. Cleanest of the PII corpora here. |
| gretelai/synthetic_pii_finance_multilingual | Apache-2.0 | yes | attribution + NOTICE | Synthetic financial documents. Clean. |
| CoNLL-2003 (`eriktks/conll2003`) | `license: other` | **restricted** | see source | Built on the Reuters RCV1 corpus, which is not freely redistributable. Tagged `source_datasets: extended|other-reuters-corpus`. Evaluation-only in practice. |
| Kaggle PII Detection | **per-competition terms** | varies | varies | Kaggle licences are set per dataset/competition and commonly forbid redistribution. Accept the competition terms; do not commit the data. |
| Financial PII (Mendeley) | not verified | unknown | unknown | Cited in SPEC.md by source only. Confirm on the Mendeley record before use. |
| SecretBench | not verified | unknown | unknown | Listed in SPEC.md; no licence recorded yet. |
| wikiann / MultiNERD / FewNERD / FiNER-139 | permissive, per-dataset | mostly yes | attribution | Reached only through PIIBench, so the composite terms above already apply. |
| dlib (planned, via go-face) | Boost Software License 1.0 | yes | keep copyright notice | Permissive; explicitly allows closed-source commercial use. |
| dlib **model weights** (iBUG LFW etc.) | **varies, some non-commercial** | restricted | varies | Separate from the library licence. Commercial face-recognition use has historically required care. Verify per weight file. |

## Recommendations

1. **Start with the clean synthetic corpora.** `nvidia/Nemotron-PII` (CC-BY 4.0)
   and `gretelai/synthetic_pii_finance_multilingual` (Apache-2.0) carry ordinary
   attribution obligations and no size thresholds. They cover most PII types this
   library detects.
2. **Avoid ai4privacy for anything commercial.** The staff-count threshold means a
   company above three people needs a paid licence, and derivatives cannot be
   sublicensed. This is the single most restrictive entry in the table and it is
   the dataset SPEC.md lists first.
3. **Treat PIIBench as unusable for commercial benchmarking until audited.** Its
   own card says constituent licences govern, and it includes two entries above
   with their own restrictions. The `apache-2.0` frontmatter is misleading.
4. **Use CoNLL-2003 and Kaggle for local evaluation only, never redistribute.**
   Both are effectively evaluation-only, which is exactly why the fetch-and-score
   pattern is the right one.
5. **Do not commit any corpus.** Attribution requirements are satisfied by
   documenting the dataset and version in the benchmark report, not by
   redistributing the data.

## Provenance of `testdata/golden/seed.json`

The committed seed set is authored in this repository from synthetic example
text. Every identifier is either reserved-for-documentation
(`example.com`, `example.net`, RFC 2606) or a well-known public test value
(`4111 1111 1111 1111`, `123-45-6789`, `192.168.10.42`). No text was copied or
derived from any corpus listed above, so the seed set carries no third-party
terms and is safe to commit under the repository's own licence.

This matters: it is the reason the CI gate can run on every commit without a
licence obligation, and the reason a future contributor can add cases without
wondering whether a dataset's share-alike clause now applies to the repo.

## Adding a dataset

1. Record it in this table with the licence, verified from the dataset card or
   repo — not from a search-result summary.
2. Add a fetch script under `scripts/` that downloads to an ignored directory.
   Do not copy data into `testdata/golden/`.
3. Cite the dataset and version in the benchmark output.
4. If any derived case must be committed, add attribution here and confirm the
   dataset's terms permit redistribution of derivatives under a compatible
   licence. If they do not, do not commit the case — keep the corpus local.