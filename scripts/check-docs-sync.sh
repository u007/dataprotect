#!/usr/bin/env bash
# Verifies that the docs/ knowledge-bundle copies still match their repo-root
# sources, and reports drift when they do not.
#
# Why this exists: docs/decisions/spec-v1.md and docs/concepts/piiscrub-usage.md
# are copies of the repo-root SPEC.md and README.md. A concurrent session once
# rewrote both roots minutes after they had been copied, leaving the bundle
# describing a package layout that the spec itself says never existed
# (per-recognizer email.go/phone.go/... vs the real regex.go). Nothing detected
# it; the stale copy was only caught by hand. This script is that detector.
#
# The bundle copies are deliberately NOT byte-identical to their sources:
# piiscrub-usage.md rewrites two relative links that would otherwise dangle
# inside docs/concepts/ (there is no SPEC.md or AGENTS.md in that directory).
# Those exact substitutions are applied to the root text before comparing, so a
# genuine content change still fails while the known link rewrite does not.
#
# Read-only: this never writes to the bundle or to the root sources. Re-syncing
# is a separate, deliberate act (re-copy the body, then re-run doc_write).
#
# Usage: scripts/check-docs-sync.sh
# Exit:  0 = every bundle copy is in sync
#        1 = drift, or a source/bundle file is missing
set -euo pipefail

cd "$(dirname "$0")/.."

fail=0

# Strip ONLY the leading YAML frontmatter block. Body `---` horizontal rules
# must survive, so this must not treat every `---` as a delimiter.
strip_frontmatter() {
  awk 'NR==1 && $0=="---" {fm=1; next} fm && $0=="---" {fm=0; next} !fm' "$1"
}

# Apply the known, intentional bundle-only link rewrites. Only README.md has
# any; every other source is compared verbatim. Each rewrite moves a link that
# would dangle inside docs/concepts/ — there is no LICENSE or DATASET_LICENSES.md
# at the bundle level, so `../../` is required to reach the repo root.
normalize() {
  local src="$1"
  if [ "$src" = "README.md" ]; then
    sed \
      -e 's|\[SPEC\.md\](SPEC\.md)|[spec-v1](../decisions/spec-v1.md)|' \
      -e 's|\[AGENTS\.md\](AGENTS\.md)|[development-conventions](development-conventions.md)|' \
      -e 's|\[LICENSE\](LICENSE)|[licence](../../LICENSE)|' \
      -e 's|\[DATASET_LICENSES\.md\](DATASET_LICENSES\.md)|[dataset licences](../../DATASET_LICENSES.md)|' \
      "$src"
  else
    cat "$src"
  fi
}

check_pair() {
  local src="$1" bundle="$2"

  if [ ! -f "$src" ]; then
    echo "MISSING source: $src" >&2
    fail=1
    return
  fi
  if [ ! -f "$bundle" ]; then
    echo "MISSING bundle: $bundle" >&2
    fail=1
    return
  fi

  if diff -q <(normalize "$src") <(strip_frontmatter "$bundle") >/dev/null; then
    echo "in sync: $src -> $bundle"
  else
    echo "DRIFT:   $src -> $bundle" >&2
    echo "         (root source is newer/different; re-copy the body and re-run doc_write)" >&2
    diff <(normalize "$src") <(strip_frontmatter "$bundle") >&2 || true
    fail=1
  fi
}

check_pair "SPEC.md"   "docs/decisions/spec-v1.md"
check_pair "README.md" "docs/concepts/piiscrub-usage.md"
check_pair "AGENTS.md" "docs/concepts/development-conventions.md"

if [ "$fail" -ne 0 ]; then
  echo
  echo "check-docs-sync: bundle is out of date" >&2
  exit 1
fi

echo
echo "check-docs-sync: all bundle copies are in sync"