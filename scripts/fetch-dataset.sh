#!/usr/bin/env bash
# Downloads benchmark corpora for local evaluation.
#
# NOTHING downloaded here is committed. Every dataset lands under an ignored
# directory (see .gitignore: /testdata/external/, /datasets/), and this script
# refuses to write anywhere git would track. That separation is the licensing
# strategy, not an accident: dataset terms constrain redistribution of DATA, so
# keeping data out of the tree is what keeps the source code's own licence
# unaffected. See DATASET_LICENSES.md.
#
# Usage:
#   scripts/fetch-dataset.sh list
#   scripts/fetch-dataset.sh info <name>
#   scripts/fetch-dataset.sh fetch <name> [--dry-run]
#   scripts/fetch-dataset.sh fetch-all --dry-run
#
# `--dry-run` prints exactly what would be downloaded and how large it is,
# without transferring anything. Use it first: some of these are multi-GB.
#
# Environment:
#   HF_TOKEN   Required only for gated or accepted-licence datasets. Export it
#              from https://huggingface.co/settings/tokens. Never commit a token
#              or write one into this repo — .gitignore covers .env.
#
# Licence warning: see `info <name>` for each dataset's terms before fetching.
# Some prohibit commercial use, some require attribution, and one (ai4privacy)
# is free only for individuals, non-profits, or organisations with 3 or fewer
# staff. Fetching a dataset does not accept its licence — you do, by using it.
set -euo pipefail

cd "$(dirname "$0")/.."

ROOT="${DATAPROTECT_DATA_DIR:-testdata/external}"

# --- registry ---------------------------------------------------------------
# name | hf repo id | licence | commercial | obligations | notes
#
# Keep this in sync with DATASET_LICENSES.md, which carries the full notes and
# sources. Verify from the dataset card before trusting a row here.
REGISTRY='
nemotron-pii|nvidia/Nemotron-PII|b70ffaf5ff39e079776134c5bf4381f00a9fd1ed|CC-BY-4.0|yes|attribution|Synthetic persona-grounded PII/PHI, 100k records, 55+ categories. Cleanest option.
gretel-pii-finance|gretelai/synthetic_pii_finance_multilingual|7b844d16738527a04264f50214cb426a4cea0897|Apache-2.0|yes|attribution+NOTICE|Synthetic full financial documents with PII spans, 56k records.
conll2003|eriktks/conll2003|3f1cce917ab38486481b062921eb137e7bd3c205|other (Reuters RCV1 derived)|RESTRICTED|see source|EVALUATION ONLY. Built on Reuters RCV1, not freely redistributable.
pii-masking-43k|ai4privacy/pii-masking-43k|7d380930dc1567a61ca861a1b4de433b9601c8dc|CUSTOM dual-tier|RESTRICTED (>3 staff)|no sublicensing of derivatives|AVOID for commercial use. Free only for individuals, non-profits, for-profits with 3 or fewer staff, or evaluation.
piibench|Pritesh-2711/pii-bench|UNKNOWN-pinned-at-fetch|apache-2.0 DECLARED, constituents govern|DEPENDS on constituents|inherits constituent terms|AUDIT BEFORE COMMERCIAL USE. Consolidates 10 datasets incl. conll2003 and ai4privacy. Its apache-2.0 tag is unreliable.
'

# field <name> <column>; columns are 1=name 2=repo 3=revision 4=licence
# 5=commercial 6=obligations 7=notes
# field <name> <column-number>
field() { printf '%s\n' "$REGISTRY" | awk -F'|' -v n="$1" -v k="$2" '$1==n {print $k}' | head -1; }
exists() { [ -n "$(field "$1" 1)" ]; }

die() { printf 'fetch-dataset: %s\n' "$1" >&2; exit 1; }

usage() { sed -n '2,28p' "$0" | sed 's/^# \{0,1\}//'; }

# Refuse to download into a path git would track. This is the invariant that
# makes the whole licensing story hold, so it is enforced here rather than
# trusted to .gitignore being correct.
assert_untracked() {
  local dir="$1"
  mkdir -p "$dir"
  if git check-ignore -q --no-index "$dir/probe.json" 2>/dev/null; then
    return 0
  fi
  die "refusing to download into $dir: git would track it.
Add an ignore rule first, e.g. '/$dir/' in .gitignore.
Rationale: committing a corpus would pull its licence terms into this repository."
}

cmd_list() {
  printf '%-20s %-45s %-14s %s\n' DATASET REPO LICENCE COMMERCIAL
  printf '%s\n' "$REGISTRY" | while IFS='|' read -r name repo _rev lic comm _; do
    [ -n "${name:-}" ] || continue
    printf '%-20s %-45s %-14s %s\n' "$name" "$repo" "$lic" "$comm"
  done
}

cmd_info() {
  local name="${1:-}"
  [ -n "$name" ] || die "usage: fetch-dataset.sh info <name> (see: list)"
  exists "$name" || die "unknown dataset '$name' (see: list)"
  printf 'dataset:    %s\n' "$name"
  printf 'repo:       https://huggingface.co/datasets/%s\n' "$(field "$name" 2)"
  printf 'revision:   %s (pinned)\n' "$(field "$name" 3)"
  printf 'licence:    %s\n' "$(field "$name" 4)"
  printf 'commercial: %s\n' "$(field "$name" 5)"
  printf 'obligations: %s\n' "$(field "$name" 6)"
  printf 'notes:      %s\n' "$(field "$name" 7)"
  printf 'lands in:   %s/%s (git-ignored)\n' "$ROOT" "$name"
  case "$(field "$name" 5)" in
    *RESTRICTED*|*DEPENDS*)
      printf '\nWARNING: this dataset is restricted. Read the dataset card before use.\n' ;;
  esac
}

cmd_fetch() {
  local name="${1:-}"
  local dry="${2:-}"
  [ -n "$name" ] || die "usage: fetch-dataset.sh fetch <name> [--dry-run]"
  exists "$name" || die "unknown dataset '$name' (see: list)"

  local repo dir rev
  repo="$(field "$name" 2)"
  rev="$(field "$name" 3)"
  dir="$ROOT/$name"
  assert_untracked "$dir"

  if ! command -v hf >/dev/null 2>&1; then
    die "the 'hf' CLI is required: pip install -U huggingface_hub"
  fi

  if [ "$dry" = "--dry-run" ]; then
    printf 'would download %s @ %s -> %s/\n' "$repo" "$rev" "$dir"
    # A dry run may legitimately fail (network, auth); that is not the download
    # itself failing, so it is reported rather than fatal.
    hf download --repo-type dataset "$repo" --revision "$rev" --dry-run --local-dir "$dir" ||
      printf '(dry run could not reach the hub; the pinned revision above is still authoritative)\n'
    return 0
  fi

  printf 'downloading %s @ %s -> %s/\n' "$repo" "$rev" "$dir"
  printf 'licence: %s | commercial: %s\n' "$(field "$name" 4)" "$(field "$name" 5)"

  # Deliberately no `|| true`. A partial download must abort the script: a
  # benchmark run over a truncated corpus would report the truncation as a
  # detector defect, which is exactly the kind of false signal this repo exists
  # to eliminate. Leaving the directory without PROVENANCE.txt makes the partial
  # state obvious to the next run.
  if ! hf download --repo-type dataset "$repo" --revision "$rev" --local-dir "$dir"; then
    rm -f "$dir/PROVENANCE.txt"
    die "download failed for $name; directory left without provenance and must not be used"
  fi

  # Provenance, so a benchmark number can be traced to an exact dataset revision.
  # Without this, "measured on Nemotron-PII" is not a citable claim.
  {
    printf 'dataset: %s\nrepo: %s\nrevision: %s\nfetched: %s\nlicence: %s\n' \
      "$name" "$repo" "$rev" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(field "$name" 4)"
  } >"$dir/PROVENANCE.txt"
  printf 'recorded revision %s in %s/PROVENANCE.txt\n' "$rev" "$dir"
  printf '\nDo not copy anything from %s into testdata/golden/.\n' "$dir"
}

cmd_fetch_all() {
  local dry="${1:-}"
  printf '%s\n' "$REGISTRY" | while IFS='|' read -r name _rest; do
    [ -n "${name:-}" ] || continue
    case "$(field "$name" 5)" in
      *RESTRICTED*|*DEPENDS*)
        printf '\n== SKIPPING %s (restricted; run: info %s)\n' "$name" "$name"
        continue ;;
    esac
    cmd_fetch "$name" "$dry" || true
  done
}

case "${1:-}" in
  list) cmd_list ;;
  info) shift; cmd_info "${1:-}" ;;
  fetch) shift; cmd_fetch "${1:-}" "${2:-}" ;;
  fetch-all) shift; cmd_fetch_all "${1:-}" ;;
  ''|-h|--help|help) usage ;;
  *) die "unknown command '$1' (try: help)" ;;
esac