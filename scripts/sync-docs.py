"""Sync the docs/ bundle copies of SPEC.md, README.md and AGENTS.md.

`docs/decisions/spec-v1.md` and `docs/concepts/development-conventions.md` are
copies of `SPEC.md` and `AGENTS.md`: the body matches, with a YAML frontmatter
header in front. This script keeps those two in sync.

`docs/concepts/piiscrub-usage.md` is NOT a copy of `README.md`. It is a
condensed usage document that shares the README's title and voice but has its
own body, so it is maintained by hand.
"""

import sys

LINK_REWRITES_USAGE = [
    ("[SPEC.md](SPEC.md)", "[spec-v1](../decisions/spec-v1.md)"),
    ("[AGENTS.md](AGENTS.md)", "[development-conventions](development-conventions.md)"),
    ("[LICENSE](LICENSE)", "[licence](../../LICENSE)"),
    ("[DATASET_LICENSES.md](DATASET_LICENSES.md)", "[dataset licences](../../DATASET_LICENSES.md)"),
]


def split_frontmatter(text):
    """Return (frontmatter, body) or (None, text) when there is no frontmatter."""
    if not text.startswith("---\n"):
        return None, text
    end = text.find("\n---\n", 3)
    if end == -1:
        return None, text
    return text[: end + 5], text[end + 5 :]


def sync(root_path, bundle_path, rewrites=()):
    with open(root_path, encoding="utf-8") as fh:
        root_body = fh.read()
    with open(bundle_path, encoding="utf-8") as fh:
        bundle_text = fh.read()

    front, bundle_body = split_frontmatter(bundle_text)
    if front is None:
        print(f"{bundle_path}: no frontmatter, refusing to overwrite", file=sys.stderr)
        return 1

    # The bundle body starts with a blank line after the frontmatter; the root
    # files do not. Strip it so the comparison is body-against-body.
    if bundle_body.startswith("\n"):
        bundle_body = bundle_body[1:]

    new_body = root_body
    for old, new in rewrites:
        new_body = new_body.replace(old, new)

    if new_body == bundle_body:
        print(f"{bundle_path}: already in sync")
        return 0

    with open(bundle_path, "w", encoding="utf-8") as fh:
        fh.write(front + "\n" + new_body)
    print(f"{bundle_path}: synced from {root_path}")
    return 0


rc = 0
rc |= sync("SPEC.md", "docs/decisions/spec-v1.md")
rc |= sync("AGENTS.md", "docs/concepts/development-conventions.md")
sys.exit(rc)
