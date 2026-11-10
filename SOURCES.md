# Sources

Where every dataset, format rule and reference in this repository came from,
how to get the current version again, and what was checked, when, and with what
result. Licence terms are in [DATASET_LICENSES.md](DATASET_LICENSES.md); this
file is about provenance and refresh.

**Last full review: 2026-10-15.** Update the "Checked" column whenever a source
is re-read, and add a row rather than overwriting an old result when a finding
changes.

## How to refresh

| Need | Command or step |
|------|-----------------|
| Corpus revisions and provenance | `scripts/fetch-dataset.sh list` shows the pinned revision per corpus. `fetch <name>` writes `PROVENANCE.txt` beside the data. |
| Re-score the corpora | `make corpus` (about 6 seconds, needs the fetched data). |
| Re-score the golden seed | `scripts/benchmark.sh` |
| Read PRADO record pages | Use this URL template, which worked for every Thai and Singapore record read: `https://www.consilium.europa.eu/prado/NL/<CODE>/index.html`. Use the `www` host, uppercase `NL`, uppercase code, and the `/index.html` suffix. The `prado.consilium.europa.eu` host redirects to the PRADO home page, and lowercase paths or paths without `index.html` return 404. The `EN` path was blank in htrcli Chrome on 2026-10-14. A Cloudflare "Just a moment..." challenge clears on its own in about 10 to 15 seconds. Do not interact with it, and do not bypass it. Open the page in a tab you opened with `htrcli tabs new` (or in the tab you already own), and leave it open. Do not navigate a tab the user has open. PRADO returns 403 to `WebFetch`. |
| Read a page without a browser | `WebFetch` works for Wikipedia and python-stdnum. It does not work for PRADO. |

Pins are commit SHAs (corpora) or dated page reads (web sources). Pin a new
revision only after re-running `make corpus` and recording the change in
SPEC.md.

## Datasets

### Final set (decided 2026-10-15)

Use this block before looking for any dataset. Measured: the two corpora and Presidio's in-scope test cases. Nothing else is fetched.

**Measured (the only corpora behind published numbers):**

| Corpus | Pinned revision | Input file (split) | Source rows | Converted | Local path |
|--------|-----------------|--------------------|-------------|-----------|------------|
| `nemotron-pii` (nvidia/Nemotron-PII, CC-BY-4.0) | `b70ffaf5ff39e079776134c5bf4381f00a9fd1ed` | `nemotron-pii/data/test-00000-of-00001.parquet` (test split), sha256 `1a4b0512ecb5370f0992d29d0f9c07351e6de13f0d7ea33bb18cecb984780247` | 100000 | first 2000 rows, file order | `testdata/external/nemotron-pii/` |
| `gretel-pii-finance` (gretelai/synthetic_pii_finance_multilingual, Apache-2.0 + NOTICE) | `7b844d16738527a04264f50214cb426a4cea0897` | `gretel-pii-finance/data/combined_test-00000-of-00001.parquet` (combined_test split only), sha256 `3cd956d9955170af36ddf48a81b35d87141c306ee208f842ce62c9c3a150c0eb` | 5596 | first 2000 rows, file order | `testdata/external/gretel-pii-finance/` |

Each local directory holds `PROVENANCE.txt` (repo, revision, fetch time, licence),
`converted/` (`cases-*.json` plus `schema.json`, with the input's SHA-256 and the
row bound), and the raw `data/`. The converter reads only the splits named above.
Gretel's other language splits and nemotron's other splits are not read, so every
figure is for that split only.

Reproduce:

```
scripts/fetch-dataset.sh list                 # pinned revisions
scripts/fetch-dataset.sh fetch nemotron-pii   # or: make download (fetches the commercially usable corpora)
scripts/fetch-dataset.sh fetch gretel-pii-finance
make corpus                                   # converts (LIMIT=2000 default; LIMIT=0 means every row) and scores
make corpus-anonymize                         # Anonymize contract over all converted cases
```

**Authored in this repository:** `testdata/golden/seed.json`, 549 cases (350 positive,
199 negative), written from synthetic text. It is the CI gate, and it carries no
third-party terms (see DATASET_LICENSES.md, "Provenance").

**Every other candidate: status, reason, and the trigger for looking again.** Re-search
for a candidate only when its trigger fires.

| Candidate | Status | Reason | Re-search trigger |
|-----------|--------|--------|-------------------|
| CoNLL-2003 (`eriktks/conll2003`) | Not fetched | Reuters RCV1 derived, restricted, evaluation only | Only if evaluation-only use is approved |
| ai4privacy `pii-masking-43k` | Not fetched | Custom dual-tier licence, staff-count limit | Only if the organisation's licence changes |
| PIIBench (`Pritesh-2711/pii-bench`) | Not fetched | Declared apache-2.0, but constituents govern (includes CoNLL-2003 and ai4privacy) | Only after an audit of its ten constituents |
| ai4privacy `openpii-1m` | Not fetched | Licence tag and declared name disagree; the LICENCE file has not been read | Before any use: read the LICENCE file |
| OpenNER 1.0 | Not fetched or used | CC-BY compilation over constituents with their own licences. Repository loads (2026-10-14) | v2 benchmark suite starts (TODO, benchmark suite) |
| Microsoft Presidio test cases (`microsoft/presidio`, MIT) | Measured, apart from the CI gate (2026-10-15). Pin `2523c7b74a469270c5c78bb253f140eafca21e31`, extracted to `testdata/external/presidio/` by `make presidio`. Ten in-scope modules, 590 cases; see SPEC.md "Presidio test cases". | Test data, not a corpus. Its in-scope modules map to types this library implements. Out of scope: NLP entities, countries without a recognizer, presidio-research (evaluation framework; its generator identities are CC BY-SA 3.0). | Re-measure when the Presidio pin changes or the card fix lands. |
| Kaggle PII Detection | Not located; not fetched | Per-competition terms | v2 benchmark suite starts |
| Financial PII (Mendeley) | Not located; no URL recorded | Named in SPEC.md by source only | v2 benchmark suite starts |
| SecretBench | Not located; no URL recorded | Named in SPEC.md; no licence recorded | v2 benchmark suite starts |
| secrets-patterns-db | Not located; no URL recorded | Named in SPEC.md (v2+ list); absent from this file until 2026-10-15 | v2 benchmark suite starts |
| CoNLL 2025 NER | Open question | SPEC.md (v2+ list) names it; the registry pins `conll2003` instead | Decide which the v2 suite uses |

### Fetched and scored

| Name | Source | Pinned revision | Licence | Used for | Fetched / checked |
|------|--------|-----------------|---------|----------|-------------------|
| `nemotron-pii` | https://huggingface.co/datasets/nvidia/Nemotron-PII | `b70ffaf5ff39e079776134c5bf4381f00a9fd1ed` | CC-BY-4.0 | Corpus scoring (2000 of 100000 records) | 2026-10-06 |
| `gretel-pii-finance` | https://huggingface.co/datasets/gretelai/synthetic_pii_finance_multilingual | `7b844d16738527a04264f50214cb426a4cea0897` | Apache-2.0 + NOTICE | Corpus scoring (2000 of 5596 records) | 2026-10-06 |

Both corpora are gitignored under `testdata/external/`. The conversion and the
schema sidecar are in `tools/corpusbench`. SPEC.md "Corpus measurement" holds the
numbers.

### Registered, restricted, not fetched

| Name | Source | Pinned revision (registry) | Licence | Status |
|------|--------|----------------------------|---------|--------|
| `conll2003` | https://huggingface.co/datasets/eriktks/conll2003 | `3f1cce917ab38486481b062921eb137e7bd3c205` | Reuters RCV1 derived, restricted | Evaluation only. Not fetched. |
| `pii-masking-43k` | https://huggingface.co/datasets/ai4privacy/pii-masking-43k | `7d380930dc1567a61ca861a1b4de433b9601c8dc` | Custom dual-tier, restricted | Avoid for commercial use. Not fetched. |
| `piibench` | https://huggingface.co/datasets/Pritesh-2711/pii-bench | not pinned at fetch | "apache-2.0" declared, constituents govern | Audit before use. Not fetched. |

### Named in SPEC.md or TODO.md, not fetched

| Name | Source | Status |
|------|--------|--------|
| OpenNER 1.0 | Repository: https://github.com/bltlab/open-ner (loads, 2026-10-14). Named in DATASET_LICENSES.md. | Not fetched or used. Listed for licence review only. |
| Kaggle PII Detection | Kaggle competition | Per-competition terms. Not fetched. |
| Financial PII (Mendeley) | Named in SPEC.md | Unverified. No URL recorded. |
| SecretBench | Named in SPEC.md | Unverified. No URL recorded. |
| ai4privacy `openpii-1m` | https://huggingface.co/datasets/ai4privacy/pii-masking-openpii-1m | Licence tag and declared name disagree. Read the LICENCE file before use. |

## Model checkpoints

| Name | Source | Pinned revision | Licence | Used for | Fetched / checked |
|------|--------|-----------------|---------|----------|-------------------|
| `Qwen3-0.6B` | https://huggingface.co/Qwen/Qwen3-0.6B | Qwen/Qwen3-0.6B (HF) | Apache-2.0 | LLM second-pass judge for weak candidates (bare MyKad) | 2026-11-09 |

Qwen3-0.6B (~0.6B total params, Apache-2.0, 119 languages incl. Malay and Chinese) is the chosen model for the optional LLM second pass. Licence verified from Qwen/Qwen3-0.6B HF LICENSE (https://huggingface.co/Qwen/Qwen3-0.6B/blob/main/LICENSE, checked 2026-11-09); training-data provenance at arxiv 2505.09388 (https://arxiv.org/abs/2505.09388). Qwen 3.7 rejected: closed-weights-only, no downloadable weights (Alibaba Cloud Model Studio qwen3-7-max, https://www.alibabacloud.com/help/en/model-studio/qwen3-7-max, checked 2026-11-09). Qwen 3.8 rejected: 2.4T MoE Max under custom Qwen3.8-Max licence, not Apache-2.0 (https://www.alibabacloud.com/help/en/model-studio/qwen3-8-max, checked 2026-11-09); 27B variant is Apache-2.0 but far too large. Qwen 4 rejected: "in training", no weights, no release date, no licence (https://qcode.cc/en/qwen-4-release-date, checked 2026-11-09).

## Format and national-ID rules

Each rule's source is listed with what it supports and what it does not.

### Singapore

| Rule | Source | Supports | Does not support | Checked |
|------|--------|----------|------------------|---------|
| NRIC/FIN check letter tables | Recorded in code and tests (`recognizers/national`). Secondary source: https://www.ngiam.net/NRIC/NRIC_numbers.pdf (loads, 2026-10-14). It gives weights (2, 7, 6, 5, 4, 3, 2), mod 11, the S/T and F/G letter tables, and an offset of 4 for T and G prefixes (d0 = 0 for S or F). | Check letter for S/T/F/G. The official ICA algorithm is not published. This PDF is a third-party write-up. | Not a primary government source. The PDF's values were read on 2026-10-14; the code was not re-compared against them in this pass. | 2026-10-14 |
| Passport shape `[EK]\d{7}[A-Z]` | Microsoft Purview, "Singapore passport number": https://learn.microsoft.com/sr-latn-rs/purview/sit-defn-singapore-passport-number | E or K, seven digits, one letter. Keyword-gated. | Says nothing on E vs K in a primary source. A second Purview copy (older) allows E only. | 2026-10-09 |
| Passport shape, legacy | ICA BioPass announcement, 2006, via NAS: https://nas.gov.sg/archivesonline/data/pdfdoc/20060725983.htm | Passport number is unique now, where it was once the NRIC number. | Digit count and K prefix. The fetched page gives no format. | 2026-10-10 |
| Passport shape, official | PRADO `SGP-AS-03001`, read through htrcli in a managed Chrome: https://www.consilium.europa.eu/prado/EN/SGP-AS-03001/index.html | "lion's head / passport number (format ANNNNNNNA)" for the official passport. | Ordinary passports. E vs K. | 2026-10-12 |
| Passport shape, ordinary 2017 | PRADO `SGP-AO-05001`, read in the NL and FR language paths: https://www.consilium.europa.eu/prado/NL/SGP-AO-05001/index.html and https://www.consilium.europa.eu/prado/fr/SGP-AO-05001/index.html | "Numbering", composition `ANNNNNNNA` (letter, seven digits, letter). External ref N14833, 64 pages, max validity 5 years. The EN and lowercase paths were blank. | E vs K. The `A` class covers both. | 2026-10-14 (htrcli) |
| Passport shape, ordinary 2006 | PRADO `SGP-AO-04001`: https://www.consilium.europa.eu/prado/en/SGP-AO-04001/index.html | Loads. No numbering entry (64 pages, e-passport 2006 model). | Number format. | 2026-10-14 (htrcli) |
| Passport shape, ordinary 2003 | PRADO `SGP-AO-03001`: https://www.consilium.europa.eu/prado/en/SGP-AO-03001/index.html | Loads. No numbering entry (96 pages). | Number format. | 2026-10-14 (htrcli) |
| Passport shape, official 2017 | PRADO `SGP-AS-05001`: https://www.consilium.europa.eu/prado/en/SGP-AS-05001/index.html | Blank in htrcli Chrome. | Everything. | 2026-10-14, blocked |
| Passport shape, MRZ | Ministry of Home Affairs written reply: https://www.mha.gov.sg/media-room/newsroom/written-reply-to-parliamentary-question-on-whether-singapore-passports-can-clearly-indicate-the-surname-of-the-passport-holder-by-mr-k-shanmugam-minister-for-home-affairs-and-minister-for-law/ | Surname in the MRZ after "PASGP". | Number format. | 2026-10-14 |
| Passport shape, Wikipedia | https://en.wikipedia.org/wiki/Singapore_passport | Biometric passport number is unique, not the NRIC. | Format. | 2026-10-14 |
| Passport shape, commercial blog | https://tidytothemax.com/what-is-passport-number-singapore/ | Nothing reliable. | Says nine characters with a type letter S, D or O; conflicts with Purview. Not authoritative. | 2026-10-14 |
| Passport shape, IDnow RIVP v1.4 Annex 2 | https://idnow.io/wp-content/uploads/IDnow_VideoIdent_PVID_Remote_Identity_Verification_Policy_v1.4_EN.pdf | Maps `SGP-AO-05001` to "Passport 2017" and `THA-AO-06001` to "Passport 2020". | Number format. Vendor's accepted-document list. | 2026-10-14 |
| Passport shape, vendor sample | Sumsub sample `E93874729W` | Eight digits. Conflicts with Purview. | Used only as a conflict note. | 2026-10-09 |
| ICA 2025 reply | https://www.ica.gov.sg/news-and-publications/newsroom/media-release/response-to--include-all-previous-passport-numbers-in-new-passport--(the-straits-times-6-may-2025) | Previous passport numbers are kept in MyICA. | No format. | 2026-10-09 |

### Malaysia

| Rule | Source | Supports | Does not support | Checked |
|------|--------|----------|------------------|---------|
| MyKad keyword list and date check | Microsoft Purview entity definitions (Malaysia identity card). No definition page URL found: a guessed `sit-defn-malaysia-identity-card-number` returned 404 on 2026-10-14, and a search found only the Malaysia passport page. | Keyword gate for bare 12-digit numbers. | Nationality. | Open: URL still to find. |
| Passport shape `[AHK]\d{8}` | Wikipedia (Malaysian passport article; URL to confirm) and Purview. PRADO `MYS-AO-03001` (2017, NL path) has no numbering composition. PRADO `MYS-AO-02001` (2010, NL path) has no numbering entry. | Letter and eight digits, keyword-gated. | PRADO confirmation of the eight-digit shape. | 2026-10-14 |

### Thailand

| Rule | Source | Supports | Does not support | Checked |
|------|--------|----------|------------------|---------|
| Citizen ID check digit (mod-11) | python-stdnum, Thai PIN: https://arthurdejong.org/python-stdnum/doc/stdnum.th.pin and source https://arthurdejong.org/git/python-stdnum/tree/stdnum/th/pin.py | Check digit and the `3100600445635` doctest. | Not an official government document. | 2026-10-09 |
| Citizen ID shape | Same as above. Vendor docs: https://docs.frankieone.com/docs/tha-national-id | Thirteen digits, hyphenated 1-4-5-2-1. | Glued IDs are documented scope (decided 2026-10-15, see SPEC.md national ID bullet). | 2026-10-09 |
| Passport number format | None found. | Nothing. | Everything. Thailand passports are not covered. | See below. |
| Passport data-page label | Wikipedia (Thai passport): https://en.wikipedia.org/wiki/Thai_passport | Thai label `หนังสือเดินทางเลขที่` for the number field. A keyword candidate only. | Format. | 2026-10-10 |
| PRADO `THA-AO-06001` (2020, current) | https://www.consilium.europa.eu/prado/NL/THA-AO-06001/index.html and the `fr` path | Booklet details. The NL and fr pages both have "Numbering" as a security-feature label with no composition. "serienummer" is a print method. | Number format. | 2026-10-14 (htrcli, NL and fr). The EN path read on 2026-10-12 showed the same label. |
| PRADO `THA-AO-02002` (2012, superseded) | https://www.consilium.europa.eu/prado/NL/THA-AO-02002/index.html | Two Numbering entries that disagree on digit count: `Samenstelling: AANNNNNNN` (data page, top-right corner, 7 digits) and `Samenstelling: AAN NNNNNNN` (serial on page 3, 8 digits, "serienummer" print method). Maximum validity 5 years; first issued 23/11/2012. | Current passport. Only the 2012 booklet. | 2026-10-14 (htrcli, NL) |
| Microsoft Purview, Japan passport number: https://learn.microsoft.com/en-us/purview/sit-defn-japan-passport-number | Two letters then seven digits, keyword-gated. | Japanese passports. | Same shape as the 2012 Thai booklet (`AANNNNNNN`). A two-letter, seven-digit rule would match Japanese passports too. | 2026-10-14 |
| PRADO `THA-AS-06001` (official, 2020) | https://www.consilium.europa.eu/prado/NL/THA-AS-06001/index.html | "Nummering" appears only as a security-feature label (laser engraving, letterpress, laser perforation). No `Samenstelling` composition. | Number format. | 2026-10-15 (htrcli, NL) |
| PRADO `THA-AD-06001` (diplomatic, 2020) | https://www.consilium.europa.eu/prado/NL/THA-AD-06001/index.html | Titled DIPLOMATIC PASSPORT. "Nummering" appears as a label only. No `Samenstelling` composition. | Number format. | 2026-10-15 (htrcli, NL) |
| PRADO `THA-AP-06001` (emergency, 2020) | https://www.consilium.europa.eu/prado/NL/THA-AP-06001/index.html | Titled EMERGENCY PASSPORT. "Nummering" appears as a label only. No `Samenstelling` composition. | Number format. | 2026-10-15 (htrcli, NL) |
| Thai PRADO records: coverage | Official, ordinary, diplomatic and emergency 2020 booklets (`AS`, `AO`, `AD`, `AP`) | All four 2020 variants have been read. None gives a number format. The 2012 booklet (`THA-AO-02002`) is the only one with a composition, and it is superseded. | Format. | 2026-10-15 |
| HL7 passport namespace | https://build.fhir.org/ig/HL7/UTG/en/NamingSystem-passportNumNS-THA.xml.html | Namespace exists. | No format. | 2026-10-10 |
| Thai MFA e-passport page | https://mfa.go.th/en/page/electronic-passport?menu=5e2013adaff2fd00c8267234 | Biometric features. | No number format. Only the citizen ID's 13 digits. | 2026-10-14 |
| US State Department, Thailand | https://travel.state.gov/content/travel/en/us-visas/Visa-Reciprocity-and-Civil-Documents-by-Country/Thailand.html | Civil documents overview. | No number format. | 2026-10-14 |
| IDnow RIVP v1.4 Annex 2 (see Singapore) | Maps `THA-AO-06001` to "Passport 2020". | Number format. | 2026-10-14 |
| Vendor passport-validation page (secondary) | Says the Thai MRZ document number is nine alphanumeric characters. | Not verified against an official source or specimen. Used only as a length note. | 2026-10-14 |
| Forum claim (no letter O) | https://ask.in.th/question/what-should-i-be-aware-of-regarding-visa-applications-and-passport-numbers-when-applying-for-a-thai-visa | Nothing reliable. | Anecdotal, disputed in the thread. | 2026-10-09 |

### General standards and references

| Source | Use | Note |
|--------|-----|------|
| ICAO Doc 9303 | MRZ and passport-number field rules | Standard. Not fetched in full. Part 3 covers the MRZ. |
| SWIFT IBAN Registry, release v101 | Country IBAN lengths (`recognizers/checksum/iban_length.go`) | swift.com returned 403 to this client, so the registry was not read directly. Read instead from python-stdnum's `stdnum/iban.dat` (https://raw.githubusercontent.com/arthurdejong/python-stdnum/master/stdnum/iban.dat), which states it is generated from that release. Length is 4 plus the sum of the BBAN element lengths. 89 countries. |
| Microsoft Purview sensitive info types | Keyword lists for Malaysia and Singapore passports | Secondary. Its keywords are the gate. |
| Wikipedia "Telephone numbers in Italy" and "Telephone numbers in Spain" | Area-code and subscriber lengths for the Italy and Spain phone branches | Fetched 2026-11-02. Shape only; no checksum. |
| `dotnet.territoriali.olinfo.it` | Mirror of Purview | Not an independent source. |
| elysiatools.com | Sample generator | Not a source. |
| Regula (blog, template database) | None used | Public pages give no format. The reference database is subscription-only. |
| HTR NControl / htrcli | Reading PRADO pages (extension and managed Chrome) | Tool, not a source. |

## Blocked sources

| Source | Problem | Route to unblock |
|--------|---------|------------------|
| PRADO record pages (EN and lowercase paths) | HTTP 403 to `WebFetch`. In htrcli Chrome the `EN`, `en`, `En` paths load blank. The `NL` and `fr` paths load (2026-10-14). | Use the `NL` or `fr` path with `htrcli tabs new`. If it is blank too, the user pastes the text from their own browser. |
| PRADO `SGP-AO-05001` EN and lowercase paths | Blank body on two attempts (2026-10-12). On 2026-10-14 the `EN`, `en`, `En` and `EN` paths were blank in htrcli Chrome, and the user reported a 404 for the link they had. Listing link does not navigate on click. | Use the `NL` or `fr` path (see Singapore table). Those load. |
| PRADO `SGP-AS-05001` | Blank on 2026-10-14. | Same as above. |
| ICAO Doc 9303 | Searched only, not fetched. Nine-character document-number field in the MRZ. | Fetch Part 3 if a length rule is ever needed. |
| Regula reference database | Subscription. | Paid access. Not planned. |
| Expert FADO / iFADO | Restricted to agencies. | Not available to this project. |

## Open questions

- Singapore passport: E vs K is accepted on Purview (E or K), and the regex takes both behind the keyword gate. The PRADO `ANNNNNNNA` shape (ordinary 2017 and official passports) confirms the digit count. Ordinary 2003 and 2006 records have no numbering entry.
- Thailand passport: no format for the 2020 booklet from any source checked (Wikipedia, MFA, US State Department, PRADO `THA-AO-06001` NL and fr, HL7, IDnow Annex 2). The 2012 booklet (`THA-AO-02002`) has two entries that disagree on digit count (7 and 8), and its shape `AANNNNNNN` matches Japan's Purview pattern. Thai passports stay uncovered (decided 2026-10-15, user, on advisor's recommendation; all four 2020 booklets read with no composition). A nine-character alphanumeric rule would match non-passport strings and is not added.
- Malaysia passport: the eight-digit shape `[AHK]\d{8}` has no PRADO composition. `MYS-AO-03001` (2017, NL) has none; `MYS-AO-02001` (2010, NL) has no numbering entry.
- Two of three URL gaps closed 2026-10-14 (OpenNER, NRIC check tables). The MyKad Purview page is still unlocated (guessed URL 404).

## Secret datasets and provider token formats (checked 2026-10-19)

These supersede the "Not located" rows for SecretBench and secrets-patterns-db in the Final set table above.

### Secret datasets: licence and access

| Source | URL | Licence | Access | Usable | Why |
|--------|-----|---------|--------|--------|-----|
| secrets-patterns-db (mazen160) | https://github.com/mazen160/secrets-patterns-db | README says CC-BY-4.0 and says the TruffleHog data is AGPL. The sidebar says CC-BY-SA-4.0. `LICENSE.md` is CC BY-SA 4.0 (raw fetch 2026-10-19). The raw `LICENSE` file returns 404 on `main` and `master`. | Public. A regex database of about 1,600 patterns, not labelled text. | No | The licence statements conflict, and ShareAlike applies to adapted material. It holds regular expressions, not labelled values, so it cannot score a value-level detector. No data fetched. |
| SecretBench (setu1421/SecretBench) | https://github.com/setu1421/SecretBench ; paper arXiv:2303.06729 | README says MIT. The LICENSE file was not read, and the page does not say whether MIT covers the dataset. | Gated. The README says researchers and tool developers must sign a data protection agreement. The data is a BigQuery dataset `dev-range-332204.secretbench.secrets` (97,479 labelled rows) and a Cloud Storage bucket `secretbench`. It contains real secrets. | No | Access needs an agreement that the project cannot sign on its own. Not downloaded. |
| CredData (Samsung/CredData) | https://github.com/Samsung/CredData | Repository is Apache-2.0 (sidebar label; LICENSE text not read). Each data file keeps its source repository's licence, and the mapping by RepoID is not shown on the page. | The data is not in the repository. `download_data.py` fetches files from the original repositories. The page says the repository mapping is for research purposes only. | No | The data licence is per file and not resolved, and the mapping terms are research-only. Not fetched. |
| Hugging Face search, labelled secret detection | WebSearch, two queries, 2026-10-19 | No dataset card with a verified MIT, Apache-2.0 or CC-BY licence was found. The one Hugging Face hit was a model card (`purebyte/secrets-code`), not a dataset. | n/a | No | No candidate met the bar. Search again when a candidate is named. |

### Provider token formats (`bareShape`)

| Format | Shape in code | Prefix source | Length and character source | Notes |
|--------|---------------|---------------|-----------------------------|-------|
| AWS access key id | `AKIA` or `ASIA`, then 16 of `[0-9A-Z]` | AWS IAM unique identifiers, prefix table: https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_identifiers.html (AKIA "Access key", ASIA "Temporary (AWS STS) access key IDs"; fetched 2026-10-19) | The AWS page gives no length. 16 characters: gitleaks `aws-access-token` rule, `[A-Z2-7]{16}` (MIT; https://raw.githubusercontent.com/gitleaks/gitleaks/master/config/gitleaks.toml). | The code uses `[0-9A-Z]`, as the brief asked, which is wider than gitleaks. |
| GitHub classic, OAuth, user-to-server, server-to-server, refresh | `gh[pousr]_`, then 36 of `[0-9A-Za-z]` | GitHub docs, "About authentication to GitHub" (https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/about-authentication-to-github): `ghp_`, `gho_`, `ghs_` named, fetched 2026-10-19. `ghu_` and `ghr_` appear in GitHub's engineering post on token formats (https://github.blog/engineering/behind-githubs-new-authentication-token-formats/), which was read only as a search excerpt. | 36 after the prefix. The GitHub docs say installation tokens are "exactly 40 characters long", which is the whole token, so 36 after `ghs_`. gitleaks `github-pat`, `github-oauth`, `github-app-token` and `github-refresh-token` use `{36}`. Checked 2026-10-21 against the engineering post itself rather than an excerpt: it names all five prefixes (`ghp`, `gho`, `ghu`, `ghs`, `ghr`) and says the redesign was done "all without changing the token length", the old formats being 40-character strings — so 36 after a 4-character prefix, for `ghu_` and `ghr_` included. | The length is 36 by the provider's own statement (2026-10-21). The code uses 36 as a FLOOR, not an exact count: the body run is consumed greedily, so a longer token is reported whole instead of being dropped by `bareBounded`, which rejects a match with a token byte after it. Measured cost of the floor on both corpora and on the seed: none — no corpus or seed value moved. |
| GitHub fine-grained PAT | `github_pat_`, then 82 of `[0-9A-Za-z_]` | Prefix named in the GitHub docs page above. | 82 characters: gitleaks `github-fine-grained-pat`, `github_pat_\w{82}`. | |
| Slack bot and user token | `xoxb-` + 10 to 13 digits + `-` + 10 to 13 digits + `-` + 24 or more alphanumerics; `xoxp-` + three digit groups + 28 to 34 alphanumerics | https://docs.slack.dev/authentication/tokens: `xoxb-` (bot) and `xoxp-` (user), fetched 2026-10-19. | Digit groups and tail: gitleaks `slack-bot-token` and `slack-user-token`. | The 24-character tail for the bot form is the brief's choice, looser than the gitleaks rule's own tail. Not checked against a provider statement. |
| Stripe live secret and restricted | `sk_live_` or `rk_live_`, then 24 or more alphanumerics | https://docs.stripe.com/keys: `sk_live_`, `rk_live_`, `sk_test_`, `pk_` named, fetched 2026-10-19. | 24 or more is the brief's figure. gitleaks `stripe-access-token` uses `{10,99}`. No provider length statement found. | Test keys are not matched (brief scope). |
| Google API key (legacy `AIza`) | `AIza`, then 35 of `[0-9A-Za-z_-]` | Google's page (https://cloud.google.com/docs/authentication/api-keys) was fetched on 2026-10-19, and the result was withheld by a content guardrail. It was fetched again on 2026-10-21 and withheld again. The Gemini API key page (https://ai.google.dev/gemini-api/docs/api-key, last updated 2026-10-06) was fetched on 2026-10-21: it documents the key TYPES but not the textual format. Prefix and length therefore still rest on gitleaks `gcp-api-key`, `AIza[\w-]{35}`. | Checked 2026-10-21. | The provider-page check found a new gap rather than confirming the format: since 2026-05-28 Google AI Studio issues "auth keys" with an `AQ.` prefix, and `AIza` is now the legacy standard/traffic key. `AIza` keys still exist, are still valid and are still matched. See "Google auth keys (`AQ.`)" below. |
| Google Gemini auth key (`AQ.`) | `AQ.`, then 35 or more of `[0-9A-Za-z_-]` | The prefix and the date come from Google itself: a staff reply on the developer forum — "We are moving away from Traffic keys (Alza) and towards a more secure Authentication Key (AQ). AI Studio will now only generate Authentication Key (AQ) keys" (https://discuss.ai.google.dev/t/account-only-generates-aq-prefix-api-keys-aiza-keys-no-longer-available-in-any-project/169242, 2026-06-12) — and the key page (https://ai.google.dev/gemini-api/docs/api-key), "Starting May 28, 2026, all new API keys created in Google AI Studio are automatically created as auth keys". The length and character set come from a merged rule in a public scanner, ai-guardian (Red Hat Product Security, v1.15.0, issue #1617): `id = "google-gemini-auth-key"`, `regex = '''\b(AQ\.[A-Za-z0-9\-_]{35,})'''`. Read from `src/ai_guardian/patterns/data/secrets.toml` on 2026-10-22. | Checked 2026-10-22. | gitleaks and TruffleHog have no `AQ.` rule at all (gitleaks master searched for `AQ\.`, `aiza`, `gemini`; TruffleHog's `googlegemini` detector is still `AIzaSy[A-Za-z0-9_-]{33}`). ai-guardian is the one public rule that encodes a length, so its floor of 35 is the floor here rather than a number of our own. A working key is 53 characters, 50 of body (NousResearch/hermes-agent issue #115356, `AQ.Ab8…`, 53 chars, twice in the same issue), which the floor covers. `AQ.` rather than `AIza` is what AI Studio now issues; the legacy shape is kept because `AIza` keys still work until September 2026. |
| npm access token | `npm_`, then 36 alphanumerics | npm changelog, 2021-09-23, "npm has a new access token format" (https://github.blog/changelog/2021-09-23-npm-has-a-new-access-token-format): the new tokens use the GitHub-style `npm_` prefix. Read as a search excerpt. | 36 after the prefix: gitleaks `npm-access-token`, `npm_[a-z0-9]{36}`. | |
| SendGrid API key | `SG.`, 22 characters, `.`, 43 characters | Twilio's support article "Can I Use a Reduced (Shorter) API Key Size in SendGrid?" (https://support.sendgrid.com/hc/en-us/articles/44146758703387), fetched 2026-10-21 via search: "SendGrid API keys are always 69 characters long. This length is fixed for all API keys generated by SendGrid", with no exception for third-party infrastructure. The API keys doc page (https://www.twilio.com/docs/sendgrid/ui/account-and-settings/api-keys, modified 2026-08-14) carries the same warning. Prefix from gitleaks `sendgrid-api-token`, `SG\.[a-z0-9=_\-\.]{66}` (MIT). | Checked 2026-10-21. 69 characters total is 3 for `SG.` + 22 + 1 for the dot + 43, which is what the code matches. Independent regex sources describe the same three parts (a 22-character key id and a 43-character token). | Confirmed by the provider (2026-10-21). The 69-character total is fixed, so a length floor would add nothing here and the exact lengths stay. |
| Bare JWT | `eyJ` header, `ey` payload, signature, each at least 8 characters | JWT format (RFC 7519, https://www.rfc-editor.org/rfc/rfc7519) not fetched. The `eyJ` prefix is the base64 of `{"`. The `ey` payload rule follows gitleaks `jwt`. | Floors of 8 are the code's choice. gitleaks uses 17 and 10. | A fourth segment makes the match unbounded and it is dropped. |

**Not added.** Slack `xoxa-`, `xoxr-`, `xoxs-`: not in Slack's docs fetched on 2026-10-19. Microsoft Purview lists them (https://learn.microsoft.com/purview/sit-defn-slack-access-token), but Purview is not the provider. Left for a decision. Stripe test keys `sk_test_` and organisation keys `sk_org_`, Slack `xoxe.`, `xapp-` and `xwfp-`, and bare JWE: outside the brief's list.

**Google auth keys (`AQ.`): found 2026-10-21, added 2026-10-22.** The gap was found by the provider-page check on 2026-10-21: Google AI Studio stopped issuing `AIza` keys, so a key created since 2026-05-28 is an "auth key" and an `AIza`-only shape matches none of them. It was left open for a day because no length was available, and inventing one is not accepted here. The length arrived from a merged public scanner rule. gitleaks and TruffleHog were searched first and have nothing: gitleaks master contains no `AQ.` alternative (its only `AQ` hit is the unrelated Yandex `AQVN` key at line 3195 of its config), and TruffleHog's `googlegemini` detector is still `AIzaSy[A-Za-z0-9_-]{33}`. ai-guardian (Red Hat Product Security) merged `google-gemini-auth-key` as `\b(AQ\.[A-Za-z0-9\-_]{35,})` in v1.15.0 (issue #1617, PR #1637, in the CHANGELOG's "30 new secret detection patterns" for Google Gemini API). Its floor of 35 is adopted as written, charset included, so the citation is exact rather than approximate. One measured key corroborates the order of magnitude: `AQ.Ab8…` at 53 characters, 50 of body, reported twice in NousResearch/hermes-agent issue #115356 by someone using it successfully against `generativelanguage.googleapis.com`. Because the body is a floor and the run is consumed greedily, a longer key is still reported whole.

**Licence of gitleaks:** MIT, checked 2026-10-19 from its raw LICENSE file.
