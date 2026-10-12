# Sources

Where every dataset, format rule and reference in this repository came from,
how to get the current version again, and what was checked, when, and with what
result. Licence terms are in [DATASET_LICENSES.md](DATASET_LICENSES.md); this
file is about provenance and refresh.

**Last full review: 2026-10-12.** Update the "Checked" column whenever a source
is re-read, and add a row rather than overwriting an old result when a finding
changes.

## How to refresh

| Need | Command or step |
|------|-----------------|
| Corpus revisions and provenance | `scripts/fetch-dataset.sh list` shows the pinned revision per corpus. `fetch <name>` writes `PROVENANCE.txt` beside the data. |
| Re-score the corpora | `make corpus` (about 6 seconds, needs the fetched data). |
| Re-score the golden seed | `scripts/benchmark.sh` |
| Read PRADO record pages | Use the `htrcli` extension transport in a tab you choose. Do not navigate a tab the user has open. PRADO returns 403 to `WebFetch`. |
| Read a page without a browser | `WebFetch` works for Wikipedia and python-stdnum. It does not work for PRADO. |

Pins are commit SHAs (corpora) or dated page reads (web sources). Pin a new
revision only after re-running `make corpus` and recording the change in
SPEC.md.

## Datasets

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
| OpenNER 1.0 | Named in DATASET_LICENSES.md | No URL recorded in this repo. Add one before any fetch. |
| Kaggle PII Detection | Kaggle competition | Per-competition terms. Not fetched. |
| Financial PII (Mendeley) | Named in SPEC.md | Unverified. No URL recorded. |
| SecretBench | Named in SPEC.md | Unverified. No URL recorded. |
| ai4privacy `openpii-1m` | https://huggingface.co/datasets/ai4privacy/pii-masking-openpii-1m | Licence tag and declared name disagree. Read the LICENCE file before use. |

## Format and national-ID rules

Each rule's source is listed with what it supports and what it does not.

### Singapore

| Rule | Source | Supports | Does not support | Checked |
|------|--------|----------|------------------|---------|
| NRIC/FIN check letter tables | Recorded only in code and tests (`recognizers/national`). The external algorithm source is not recorded here. | Check letter for S/T/F/G. | Not a primary government source in this repo. | Add a source link on next review. |
| Passport shape `[EK]\d{7}[A-Z]` | Microsoft Purview, "Singapore passport number": https://learn.microsoft.com/sr-latn-rs/purview/sit-defn-singapore-passport-number | E or K, seven digits, one letter. Keyword-gated. | Says nothing on E vs K in a primary source. A second Purview copy (older) allows E only. | 2026-10-09 |
| Passport shape, legacy | ICA BioPass announcement, 2006, via NAS: https://nas.gov.sg/archivesonline/data/pdfdoc/20060725983.htm | Passport number is unique now, where it was once the NRIC number. | Digit count and K prefix. The fetched page gives no format. | 2026-10-10 |
| Passport shape, official | PRADO `SGP-AS-03001`, read through htrcli in a managed Chrome: https://www.consilium.europa.eu/prado/EN/SGP-AS-03001/index.html | "lion's head / passport number (format ANNNNNNNA)" for the official passport. | Ordinary passports. E vs K. | 2026-10-12 |
| Passport shape, ordinary 2017 | PRADO `SGP-AO-05001`: https://www.consilium.europa.eu/prado/EN/SGP-AO-05001/index.html | Not yet read. Two attempts gave a blank body. | Everything. | Open. |
| Passport shape, vendor sample | Sumsub sample `E93874729W` | Eight digits. Conflicts with Purview. | Used only as a conflict note. | 2026-10-09 |
| ICA 2025 reply | https://www.ica.gov.sg/news-and-publications/newsroom/media-release/response-to--include-all-previous-passport-numbers-in-new-passport--(the-straits-times-6-may-2025) | Previous passport numbers are kept in MyICA. | No format. | 2026-10-09 |

### Malaysia

| Rule | Source | Supports | Does not support | Checked |
|------|--------|----------|------------------|---------|
| MyKad keyword list and date check | Microsoft Purview entity definitions (Malaysia identity card). URL not recorded here. | Keyword gate for bare 12-digit numbers. | Nationality. | Add URL on next review. |
| Passport shape `[AHK]\d{8}` | Wikipedia (Malaysian passport article; URL to confirm) and Purview. PRADO `mys` record is unverified (403). | Letter and eight digits, keyword-gated. | PRADO confirmation. | 2026-10-09 |

### Thailand

| Rule | Source | Supports | Does not support | Checked |
|------|--------|----------|------------------|---------|
| Citizen ID check digit (mod-11) | python-stdnum, Thai PIN: https://arthurdejong.org/python-stdnum/doc/stdnum.th.pin and source https://arthurdejong.org/git/python-stdnum/tree/stdnum/th/pin.py | Check digit and the `3100600445635` doctest. | Not an official government document. | 2026-10-09 |
| Citizen ID shape | Same as above. Vendor docs: https://docs.frankieone.com/docs/tha-national-id | Thirteen digits, hyphenated 1-4-5-2-1. | Glued-ID scope (a product decision). | 2026-10-09 |
| Passport number format | None found. | Nothing. | Everything. Thailand passports are not covered. | See below. |
| Passport data-page label | Wikipedia (Thai passport): https://en.wikipedia.org/wiki/Thai_passport | Thai label `หนังสือเดินทางเลขที่` for the number field. A keyword candidate only. | Format. | 2026-10-10 |
| PRADO `THA-AO-06001` | https://www.consilium.europa.eu/prado/EN/THA-AO-06001/index.html | Booklet details. "Numbering" appears only as a security-feature label. | Number format. | 2026-10-12 (htrcli) |
| PRADO `THA-AS-06001`, `THA-AD-06001`, `THA-AP-06001` | Same PRADO site | Official, diplomatic and emergency booklets. | Not read. | Open. |
| HL7 passport namespace | https://build.fhir.org/ig/HL7/UTG/en/NamingSystem-passportNumNS-THA.xml.html | Namespace exists. | No format. | 2026-10-10 |
| Forum claim (no letter O) | https://ask.in.th/question/what-should-i-be-aware-of-regarding-visa-applications-and-passport-numbers-when-applying-for-a-thai-visa | Nothing reliable. | Anecdotal, disputed in the thread. | 2026-10-09 |

### General standards and references

| Source | Use | Note |
|--------|-----|------|
| ICAO Doc 9303 | MRZ and passport-number field rules | Standard. Not fetched in full. Part 3 covers the MRZ. |
| Microsoft Purview sensitive info types | Keyword lists for Malaysia and Singapore passports | Secondary. Its keywords are the gate. |
| `dotnet.territoriali.olinfo.it` | Mirror of Purview | Not an independent source. |
| elysiatools.com | Sample generator | Not a source. |
| Regula (blog, template database) | None used | Public pages give no format. The reference database is subscription-only. |
| HTR NControl / htrcli | Reading PRADO pages (extension and managed Chrome) | Tool, not a source. |

## Blocked sources

| Source | Problem | Route to unblock |
|--------|---------|------------------|
| PRADO record pages | HTTP 403 to `WebFetch`. htrcli managed Chrome reads some pages. The extension transport cannot open a tab that is not the user's. | The user opens the URL and pastes the text, or names a tab htrcli may navigate. |
| PRADO `SGP-AO-05001` | Blank body on two attempts (2026-10-12). | Retry from a browser the user opens. |
| Regula reference database | Subscription. | Paid access. Not planned. |
| Expert FADO / iFADO | Restricted to agencies. | Not available to this project. |

## Open questions

- Singapore passport: E vs K, and digit count. The official `ANNNNNNNA` confirms the shape for AS passports only. The ordinary passport record is unread.
- Thailand passport: no format from any source. Needs a specimen or PRADO `THA-AO-06001` numbering text read by someone with browser access. It is not confirmed that the PRADO "Numbering" entries carry a format anywhere.
- Malaysia passport: the PRADO `mys` record is unverified.
- Three sources have no URL recorded here (NRIC check tables, MyKad Purview page, OpenNER). Add them when next checked.
