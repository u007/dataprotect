#!/usr/bin/env python3
"""Check that README.md and docs/concepts/piiscrub-usage.md stay in sync with
testdata/golden/seed.json.

Both documents repeat, by hand, facts that belong to the seed set:
- total / positive / negative case counts
- aggregate precision / recall / F1 and tp / fp / fn (exact and overlap)
- per-type coverage counts

This script loads seed.json as the source of truth, extracts the claimed
numbers from both documents, and exits non-zero on any mismatch. Add it to CI
(or run `make docs-check`) so a seed-set change can't silently leave a stale
number in a document.

Usage:

    python3 scripts/docs_check.py
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SEED = ROOT / "testdata/golden/seed.json"
README = ROOT / "README.md"
USAGE = ROOT / "docs/concepts/piiscrub-usage.md"


def fail(message):
    print(f"docs_check: {message}", file=sys.stderr)
    sys.exit(1)


def load_seed(path=SEED):
    data = json.loads(path.read_text(encoding="utf-8"))
    cases = data["cases"] if isinstance(data, dict) and "cases" in data else data
    total = len(cases)
    positive = sum(1 for c in cases if c.get("entities"))
    negative = total - positive
    per_type = {}
    for c in cases:
        for e in c.get("entities", []):
            per_type[e["type"]] = per_type.get(e["type"], 0) + 1
    return {
        "total": total,
        "positive": positive,
        "negative": negative,
        "per_type": per_type,
    }


# Aggregate metrics are measured by golden/accuracy_test.go and are stable
# unless the seed or the detector changes. They are captured here as the
# expected values, cross-checked against the seed counts below.
AGGREGATE = {
    "total": 553,
    "positive": 353,
    "negative": 200,
    "tp": 384,
    "fp": 0,
    "fn": 1,
    "precision": "1.0000",
    "recall": "0.9974",
    "f1": "0.9987",
}

# Per-type coverage expected values, computed from seed.json by hand and
# recorded here so the script can compare against both documents. This is the
# single place to update when the seed changes.
PER_TYPE = {
    "BITCOIN": 13,
    "CREDIT_CARD": 56,
    "EMAIL": 57,
    "IBAN": 35,
    "IP_ADDRESS": 46,
    "MY_IC": 3,
    "MY_PASSPORT": 3,
    "PHONE": 58,
    "SECRET": 37,
    "SG_NRIC": 5,
    "SG_PASSPORT": 2,
    "SSN": 36,
    "TH_ID": 4,
    "URL": 30,
}

# Entity types that exist in recognizers.AllEntityTypes. NLP types
# (PERSON, ORGANIZATION, LOCATION) are not in the seed and are excluded.
SEED_TYPES = set(PER_TYPE)


def read(path):
    return path.read_text(encoding="utf-8")


def extract_readme_numbers(text):
    """Return a dict of claimed numbers from README.md's 'Headline results'
    section and the surrounding measured-results prose."""
    claims = {}

    m = re.search(
        r"Golden seed \| (\d+) \| exact span \| ([\d.]+) \| ([\d.]+) \| ([\d.]+) \| (\d+) / (\d+) / (\d+) \|",
        text,
    )
    if m:
        claims["readme_exact_cases"] = int(m.group(1))
        claims["readme_exact_p"] = m.group(2)
        claims["readme_exact_r"] = m.group(3)
        claims["readme_exact_f1"] = m.group(4)
        claims["readme_exact_tp"] = int(m.group(5))
        claims["readme_exact_fp"] = int(m.group(6))
        claims["readme_exact_fn"] = int(m.group(7))

    m = re.search(
        r"Golden seed \| (\d+) \| byte overlap \| ([\d.]+) \| ([\d.]+) \| ([\d.]+) \| (\d+) / (\d+) / (\d+) \|",
        text,
    )
    if m:
        claims["readme_overlap_cases"] = int(m.group(1))
        claims["readme_overlap_p"] = m.group(2)
        claims["readme_overlap_r"] = m.group(3)
        claims["readme_overlap_f1"] = m.group(4)
        claims["readme_overlap_tp"] = int(m.group(5))
        claims["readme_overlap_fp"] = int(m.group(6))
        claims["readme_overlap_fn"] = int(m.group(7))

    m = re.search(r"seed (\d+) of (\d+) negative", text)
    if m:
        claims["readme_neg_clean"] = int(m.group(1))
        claims["readme_neg_total"] = int(m.group(2))

    return claims


def extract_usage_numbers(text):
    """Return a dict of claimed numbers from docs/concepts/piiscrub-usage.md."""
    claims = {}

    m = re.search(r"baseline over (\d+) cases: exact-span P=([\d.]+) R=([\d.]+)", text)
    if m:
        claims["usage_cases"] = int(m.group(1))
        claims["usage_p"] = m.group(2)
        claims["usage_r"] = m.group(3)

    m = re.search(r"holds (\d+) hand-labelled cases \((\d+) positive, (\d+) negative\)", text)
    if m:
        claims["usage_total"] = int(m.group(1))
        claims["usage_positive"] = int(m.group(2))
        claims["usage_negative"] = int(m.group(3))

    # Coverage table: | EMAIL | 51 |
    for m in re.finditer(r"\|\s*(BITCOIN|CREDIT_CARD|EMAIL|IBAN|IP_ADDRESS|MY_IC|MY_PASSPORT|PHONE|SECRET|SG_NRIC|SG_PASSPORT|SSN|TH_ID|URL)\s*\|\s*(\d+)[^|]*\|", text):
        claims[f"usage_type_{m.group(1)}"] = int(m.group(2))

    # Aggregate table rows: | exact span | 1.0000 | 0.9974 | 0.9987 | 384/0/1 |
    m = re.search(r"exact span \| ([\d.]+) \| ([\d.]+) \| ([\d.]+) \| (\d+)/(\d+)/(\d+) \|", text)
    if m:
        claims["usage_exact_p"] = m.group(1)
        claims["usage_exact_r"] = m.group(2)
        claims["usage_exact_f1"] = m.group(3)
        claims["usage_exact_tp"] = int(m.group(4))
        claims["usage_exact_fp"] = int(m.group(5))
        claims["usage_exact_fn"] = int(m.group(6))

    return claims


def main():
    seed = load_seed()
    errors = []

    # 1. Seed counts must match the recorded expected values (sanity that the
    # script itself isn't stale).
    if seed["total"] != AGGREGATE["total"]:
        errors.append(f"seed total {seed['total']} != expected {AGGREGATE['total']}")
    if seed["positive"] != AGGREGATE["positive"]:
        errors.append(f"seed positive {seed['positive']} != expected {AGGREGATE['positive']}")
    if seed["negative"] != AGGREGATE["negative"]:
        errors.append(f"seed negative {seed['negative']} != expected {AGGREGATE['negative']}")
    for typ, expected in PER_TYPE.items():
        actual = seed["per_type"].get(typ, 0)
        if actual != expected:
            errors.append(f"seed {typ} {actual} != expected {expected}")

    readme = read(README)
    usage = read(USAGE)
    r = extract_readme_numbers(readme)
    u = extract_usage_numbers(usage)

    # 2. README headline table vs expected aggregates.
    for key, expected in (
        ("readme_exact_cases", AGGREGATE["total"]),
        ("readme_exact_p", AGGREGATE["precision"]),
        ("readme_exact_r", AGGREGATE["recall"]),
        ("readme_exact_f1", AGGREGATE["f1"]),
        ("readme_exact_tp", AGGREGATE["tp"]),
        ("readme_exact_fp", AGGREGATE["fp"]),
        ("readme_exact_fn", AGGREGATE["fn"]),
        ("readme_overlap_cases", AGGREGATE["total"]),
        ("readme_overlap_p", AGGREGATE["precision"]),
        ("readme_overlap_r", AGGREGATE["recall"]),
        ("readme_overlap_f1", AGGREGATE["f1"]),
        ("readme_overlap_tp", AGGREGATE["tp"]),
        ("readme_overlap_fp", AGGREGATE["fp"]),
        ("readme_overlap_fn", AGGREGATE["fn"]),
    ):
        if key in r and r[key] != expected:
            errors.append(f"README {key}: {r[key]} != expected {expected}")

    # 3. README negative-cases sentence.
    if "readme_neg_clean" in r:
        if r["readme_neg_clean"] != AGGREGATE["negative"]:
            errors.append(f"README neg clean {r['readme_neg_clean']} != {AGGREGATE['negative']}")
        if r["readme_neg_total"] != AGGREGATE["negative"]:
            errors.append(f"README neg total {r['readme_neg_total']} != {AGGREGATE['negative']}")

    # 4. Usage doc vs expected aggregates.
    for key, expected in (
        ("usage_cases", AGGREGATE["total"]),
        ("usage_p", AGGREGATE["precision"]),
        ("usage_r", AGGREGATE["recall"]),
        ("usage_total", AGGREGATE["total"]),
        ("usage_positive", AGGREGATE["positive"]),
        ("usage_negative", AGGREGATE["negative"]),
        ("usage_exact_p", AGGREGATE["precision"]),
        ("usage_exact_r", AGGREGATE["recall"]),
        ("usage_exact_f1", AGGREGATE["f1"]),
        ("usage_exact_tp", AGGREGATE["tp"]),
        ("usage_exact_fp", AGGREGATE["fp"]),
        ("usage_exact_fn", AGGREGATE["fn"]),
    ):
        if key in u and u[key] != expected:
            errors.append(f"usage {key}: {u[key]} != expected {expected}")

    # 5. Usage per-type table vs seed counts.
    for typ, expected in PER_TYPE.items():
        key = f"usage_type_{typ}"
        if key in u and u[key] != expected:
            errors.append(f"usage {typ} {u[key]} != expected {expected}")

    # 6. Every seed type must appear in the usage coverage table.
    for typ in SEED_TYPES:
        if f"usage_type_{typ}" not in u:
            errors.append(f"usage doc missing coverage row for {typ}")

    if errors:
        print("docs_check: DRIFT DETECTED:", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        sys.exit(1)

    print("docs_check: README.md and docs/concepts/piiscrub-usage.md are in sync with testdata/golden/seed.json")


if __name__ == "__main__":
    main()
