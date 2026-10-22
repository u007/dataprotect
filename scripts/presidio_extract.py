#!/usr/bin/env python3
"""Convert Presidio's in-scope recognizer test cases into golden case dirs.

Usage (run isolated, never from inside the clone):

    python3 -I scripts/presidio_extract.py <presidio-clone> <out-dir> <revision>

For each module in MODULES it writes <out-dir>/<stem>/converted/ holding
cases-000000.json and schema.json, in the format golden.Load and
golden.LoadSchema read. scripts/benchmark.sh scores each directory. Rows that
cannot be turned into gold spans are written to <out-dir>/<stem>/skipped.json,
with the reason, outside converted/ so the scorer never loads them.

The clone is read, never imported or executed. Only the `parametrize` lists are
parsed. Any row or column this script does not recognise is an error, never a
silent skip.

Presidio offsets are Python character indices. The golden format is UTF-8
byte offsets, so each span is converted and its value is taken from the text.
"""

import ast
import hashlib
import json
import sys
from pathlib import Path

# Presidio test modules. Only types this library implements are listed. NLP
# entities (PERSON, LOCATION, ...) and countries without a recognizer here are
# out of scope and are listed in SOURCES.md.
MODULES = [
    "test_email_recognizer.py",
    "test_ip_recognizer.py",
    "test_url_recognizer.py",
    "test_iban_recognizer.py",
    "test_crypto_recognizer.py",
    "test_credit_card_recognizer.py",
    "test_phone_recognizer.py",
    "test_us_ssn_recognizer.py",
    "test_th_tnin_recognizer.py",
    "test_sg_fin_recognizer.py",
]

# Presidio entity type -> this library's entity type.
ENTITY_MAP = {
    "EMAIL_ADDRESS": "EMAIL",
    "IP_ADDRESS": "IP_ADDRESS",
    "URL": "URL",
    "IBAN_CODE": "IBAN",
    "CRYPTO": "BITCOIN",
    "CREDIT_CARD": "CREDIT_CARD",
    "PHONE_NUMBER": "PHONE",
    "US_SSN": "SSN",
    "TH_TNIN": "TH_ID",
    "SG_NRIC_FIN": "SG_NRIC",
}

TEXT_COLUMNS = {"text", "iban"}
POSITION_COLUMNS = {"expected_positions", "expected_res"}
ENTITY_LIST_COLUMN = "entities"
LEN_COLUMN = "expected_len"
LENIENCY_COLUMN = "leniency"
# Score columns carry Presidio's own confidence, which this scorer does not use.
SCORE_COLUMNS = {
    "expected_score",
    "expected_scores",
    "expected_score_ranges",
    "score",
    "expected_textual_explanations",
}
ALLOWED_COLUMNS = (
    TEXT_COLUMNS
    | POSITION_COLUMNS
    | SCORE_COLUMNS
    | {ENTITY_LIST_COLUMN, LEN_COLUMN, LENIENCY_COLUMN}
)


def fail(where, message):
    raise SystemExit(f"presidio_extract: {where}: {message}")


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def fixture_entity(path, tree):
    """Return the Presidio entity from an `entities` fixture, or None if absent."""
    for node in tree.body:
        if isinstance(node, ast.FunctionDef) and node.name == "entities":
            returns = [n for n in ast.walk(node) if isinstance(n, ast.Return)]
            if len(returns) != 1:
                fail(path, "entities fixture must have exactly one return")
            value = ast.literal_eval(returns[0].value)
            if not isinstance(value, list) or len(value) != 1:
                fail(path, f"entities fixture must return one type, got {value!r}")
            return value[0]
    return None


def literal(node, where):
    """ast.literal_eval, plus f-strings with no interpolation (Presidio uses them)."""
    if isinstance(node, ast.JoinedStr) and all(isinstance(v, ast.Constant) for v in node.values):
        return "".join(v.value for v in node.values)
    if isinstance(node, ast.Tuple):
        return tuple(literal(e, where) for e in node.elts)
    if isinstance(node, ast.List):
        return [literal(e, where) for e in node.elts]
    if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Mult):
        # `[x] * 2`, used for repeated score ranges.
        left, right = literal(node.left, where), literal(node.right, where)
        if isinstance(left, list) and isinstance(right, int) and not isinstance(right, bool):
            return left * right
        fail(where, "only list * int is supported")
    try:
        return ast.literal_eval(node)
    except ValueError as exc:
        fail(where, f"not a literal: {exc}")


def parametrize_calls(func):
    for dec in func.decorator_list:
        if (
            isinstance(dec, ast.Call)
            and isinstance(dec.func, ast.Attribute)
            and dec.func.attr == "parametrize"
        ):
            yield dec


def to_bytes(text, char_index):
    return len(text[:char_index].encode("utf-8"))


def convert_row(where, fixture_type, names, row, seen_types, emitted_texts):
    """Return (text, labels), or (text, None, reason) when the row is set aside."""
    if not isinstance(row, tuple) or len(row) != len(names):
        fail(where, f"row does not have {len(names)} values: {row!r}")
    cols = dict(zip(names, row))

    unknown = set(names) - ALLOWED_COLUMNS
    if unknown:
        fail(where, f"unrecognised column(s) {sorted(unknown)}")

    text_cols = [n for n in names if n in TEXT_COLUMNS]
    pos_cols = [n for n in names if n in POSITION_COLUMNS]
    if len(text_cols) != 1 or len(pos_cols) != 1 or LEN_COLUMN not in cols:
        fail(where, "row needs one text column, one position column and expected_len")
    text = cols[text_cols[0]]
    positions = cols[pos_cols[0]]
    count = cols[LEN_COLUMN]
    if not isinstance(text, str):
        fail(where, f"text is not a string: {text!r}")
    if not isinstance(count, int) or isinstance(count, bool):
        fail(where, f"expected_len is not an int: {count!r}")
    if not isinstance(positions, (tuple, list)):
        fail(where, f"positions is not a tuple or list: {positions!r}")

    if ENTITY_LIST_COLUMN in cols:
        types = cols[ENTITY_LIST_COLUMN]
        if not isinstance(types, (list, tuple)) or len(types) != count:
            fail(where, f"entities {types!r} does not match expected_len {count}")
    else:
        if fixture_type is None:
            fail(where, "no entity type from an entities fixture or column")
        types = [fixture_type] * count
    for t in types:
        if t not in ENTITY_MAP:
            fail(where, f"entity {t!r} has no mapping in ENTITY_MAP")
        seen_types.add(t)

    # golden.Load rejects empty text, so the empty-string negative is set aside.
    if text == "":
        return text, None, "empty text: golden format requires non-empty text"
    # The same text can appear twice within one module: once bare and once in
    # context, or once for the default recognizer and once for a leniency
    # variant. The second row is a second opinion on the same text, not a new
    # case, so it is set aside (the converted case keeps its original id;
    # emitting both would renumber every later case). A leniency or bare row
    # for NEW text converts like any other row, so no coverage is dropped.
    if text in emitted_texts:
        return text, None, f"duplicate text: already converted from {emitted_texts[text]}"
    # Presidio asserts how many matches exist but not where. Guessing a span
    # would invent gold, so these rows are set aside — unless the row asserts
    # exactly one match on text that is a single whitespace-free token. Then
    # there is exactly one span the gold could be, the whole text, and writing
    # it is transcription, not invention. Anything else (count 0, count 2+,
    # text with surrounding context) stays set aside.
    if count > 0 and not positions:
        if count == 1 and len(text.split()) == 1:
            single = ENTITY_MAP[fixture_type] if fixture_type is not None else ENTITY_MAP[types[0]]
            return text, [{"type": single, "value": text, "start": 0, "end": len(text.encode("utf-8"))}], None
        return text, None, "count only: Presidio asserts no span"

    if len(positions) != count:
        fail(where, f"expected_len {count} does not match positions {positions!r}")
    labels = []
    for span, t in zip(positions, types):
        if not (isinstance(span, tuple) and len(span) == 2 and all(isinstance(v, int) for v in span)):
            fail(where, f"span is not an (int, int) pair: {span!r}")
        start, end = span
        if not (0 <= start < end <= len(text)):
            fail(where, f"span {span} out of bounds for text of length {len(text)}")
        labels.append(
            {
                "type": ENTITY_MAP[t],
                "value": text[start:end],
                "start": to_bytes(text, start),
                "end": to_bytes(text, end),
            }
        )
    labels.sort(key=lambda label: (label["start"], label["end"]))
    return text, labels, None


def overlapping_pairs(labels):
    count = 0
    for i, a in enumerate(labels):
        for b in labels[i + 1 :]:
            if a["start"] < b["end"] and b["start"] < a["end"]:
                count += 1
    return count


def convert_module(src, out, revision, name):
    path = src / "presidio-analyzer" / "tests" / name
    rel = path.relative_to(src).as_posix()
    tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    fixture_type = fixture_entity(path, tree)
    if fixture_type is not None and fixture_type not in ENTITY_MAP:
        fail(rel, f"entities fixture {fixture_type!r} has no mapping in ENTITY_MAP")

    cases, skipped, seen_types, per_function = [], [], set(), []
    emitted_texts = {}
    for func in [n for n in tree.body if isinstance(n, ast.FunctionDef)]:
        for dec in parametrize_calls(func):
            if len(dec.args) < 2:
                fail(f"{rel}:{dec.lineno}", "parametrize without names and values")
            names_node = dec.args[0]
            if not (isinstance(names_node, ast.Constant) and isinstance(names_node.value, str)):
                fail(f"{rel}:{dec.lineno}", "parametrize names are not a literal string")
            names = [n.strip() for n in names_node.value.split(",")]
            if not isinstance(dec.args[1], ast.List):
                fail(f"{rel}:{dec.lineno}", "parametrize values are not a list literal")
            found = 0
            for element in dec.args[1].elts:
                where = f"{rel}:{element.lineno}"
                row = literal(element, where)
                text, labels, reason = convert_row(where, fixture_type, names, row, seen_types, emitted_texts)
                if labels is None:
                    skipped.append({"note": where, "reason": reason, "text": text})
                    continue
                emitted_texts[text] = where
                cases.append(
                    {"id": f"{path.stem}-{len(cases):04d}", "note": where, "text": text, "entities": labels}
                )
                found += 1
            per_function.append((func.name, found))

    for fname, found in per_function:
        print(f"  {fname}: {found} cases")
    if not seen_types:
        fail(rel, "no entity types found")

    target = out / path.stem / "converted"
    target.mkdir(parents=True, exist_ok=True)
    (target / "cases-000000.json").write_text(
        json.dumps(cases, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    negatives = sum(1 for c in cases if not c["entities"])
    overlaps = sum(overlapping_pairs(c["entities"]) for c in cases)
    rows_total = len(cases) + len(skipped)
    schema = {
        "corpus": f"presidio-{path.stem}",
        "source": rel,
        "source_sha256": sha256(path),
        "source_revision": revision,
        "source_rows": rows_total,
        "limit": 0,
        "cases": len(cases),
        "offset_unit": "bytes",
        "unlabelled_recorded": True,
        "in_schema": sorted({ENTITY_MAP[t] for t in seen_types}),
        "unmapped_labels": [],
        "overlapping_pairs": overlaps,
        "negatives": negatives,
        "generated_by": "scripts/presidio_extract.py",
    }
    (target / "schema.json").write_text(json.dumps(schema, indent=2) + "\n", encoding="utf-8")
    if skipped:
        (out / path.stem / "skipped.json").write_text(
            json.dumps(skipped, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
    print(
        f"{path.stem}: {len(cases)} cases, {negatives} negatives, {overlaps} overlapping pairs, "
        f"{len(skipped)} set aside -> {target}"
    )


def main(argv):
    if len(argv) != 4:
        raise SystemExit(__doc__)
    src, out, revision = Path(argv[1]), Path(argv[2]), argv[3]
    if len(revision) != 40:
        raise SystemExit("presidio_extract: revision must be a full 40-character commit SHA")
    for name in MODULES:
        convert_module(src, out, revision, name)


if __name__ == "__main__":
    main(sys.argv)
