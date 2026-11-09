---
name: review-map
description: >-
  review-map: append a machine-readable block to a Gitea PR body that maps
  each description bullet to the exact diff lines implementing it. Use when
  authoring a PR with more than ~200 changed lines on an instance with the
  Review Map feature, or when updating one after new commits (invalid
  references, unmapped files, or an ⚠ badge mean the block drifted).
---

# Review Map: make your PR reviewable bullet by bullet

Humans cannot review a 3,000-line AI-generated PR as one wall of code. A
review-map block turns each bullet of the PR description into server-validated
links to the exact diff lines that implement it, with coverage math and
per-bullet review progress. This file is the exact contract the server's
strict parser enforces.

**Done when:** the Review Map panel reads `✓ verified` with zero invalid
references, zero orphans, and the unmapped set is exactly your deliberate
`ignore` list. Everything below serves that gate.

## When to write a review map

- Any PR with more than ~200 changed lines. Skip it for trivial PRs — a PR
  without a block renders exactly like any other Gitea PR.
- Write the human-readable description FIRST, as a top-level dash list
  (`- item`). Each top-level list item is one review unit; nested items are not.
- Then append ONE review-map block after the description.

## Complete example PR body

Copy this shape, replace the content:

````markdown
## Summary

- Added Advanced Query slice and selectors to manage state and criteria
- Updated infinite row datasource to support fetching data from the Advanced Query endpoint

```review-map
ignore:
  - "package-lock.json"
  - "src/**/*.snap"
bullets:
  - bullet: "Added Advanced Query slice and selectors to manage state and criteria"
    summary: "A small store slice holds the filter criteria; selectors read it without re-rendering the grid."
    changes:
      - file: src/store/slices/AdvancedQuery.ts
        lines: 1-120
        note: "The slice itself: criteria state plus the reducer cases."
      - file: src/store/selectors/AdvancedQuery.ts
        lines: 1-45
  - bullet: "Updated infinite row datasource to support fetching data from the Advanced Query endpoint"
    changes:
      - file: src/utilities/infiniteRowDatasource.ts
        lines: 30-88
```
````

Anatomy of the block:

- The fence line is ` ```review-map ` at the very start of a line (trailing
  spaces after the info string are tolerated, leading whitespace is not); the
  block ends at the next line starting with ` ``` `. The FIRST such block in
  the body wins; later ones are ignored.
- Top level: `ignore:` and `bullets:` — either order, each at most once, never
  a value after the colon. No other keys, no comments.
- `ignore:` entries are double-quoted glob strings.
- Each bullet: `- bullet: "<exact text>"` with an optional `changes:` list of
  `- file: <path>` plus `lines:`.
- Indentation is exact 2-space steps (2 for list items, 4 for `changes:`,
  6 for `- file:`, 8 for `lines:`). Blank lines are allowed; tabs never.

## Rules

Format:

- `bullet:` and `ignore:` values MUST be double-quoted strings, and the text
  must not contain a double quote — there is no escape syntax. Write the
  description bullets without double quotes in the first place.
- `bullet:` text must be a byte-exact copy of a top-level description bullet,
  and that bullet must sit outside any code fence (the server skips fenced
  content when reading the description — a bullet list inside a fence matches
  nothing). A mismatch makes the entry an "orphan": it still validates, but
  the panel shows a warning.
- The plain-English layer is optional but recommended: `summary:` (one quoted
  sentence, max 280 chars) per bullet, `note:` (max 280 chars) after a file
  entry's `lines:` explaining THAT file's part, and `details:` (up to 8 quoted
  lines of max 200 chars) for background. Notes are what reviewers read above
  the mapped lines — prefer a specific per-file note over vague change-level
  prose. Over-length or misplaced keys reject the whole block.
- `file:` is a repo-relative path with forward slashes only: no leading `/`,
  no `..` segment, no backslash, at most 500 characters. Quote it if it
  contains spaces.
- `lines:` is `N` or `N-M` with `1 <= N <= M`, no spaces inside the value.
- `ignore:` globs are gitignore-style, matched against `/`-separated paths:
  `*` stays within one segment, `**` crosses segments.

Limits (instance defaults; an admin can change them via `[review_map]` in
`app.ini`): at most 50 bullets, at most 10 file entries per bullet, and the
whole block at most 4 KB.

Semantics:

- Line numbers are NEW-side (post-change) numbers, inclusive on both ends —
  the right-hand gutter of the diff view.
- A range is valid iff its file is in the diff AND the range intersects at
  least one changed hunk. Ranges MAY include unchanged context lines around a
  change — do so, reviewers need context; context lines just don't count
  toward coverage.
- One bullet may map to many files, and ranges may overlap.
- Coverage counts only changed lines inside mapped ranges. Every changed file
  must be mapped by some bullet or listed in `ignore`, or it shows up as
  unmapped.
- `ignore` removes files from coverage and validation entirely (they stay
  visible in Files Changed). NEVER reference an ignored file from a bullet —
  references to ignored files show as invalid.
- Files that cannot be line-mapped go in `ignore`, not in bullets: deleted
  files (no new-side lines exist), binary files, submodules, plus lockfiles
  and vendored/generated files you touched.

## Common mistakes

### Parse-breaking: the ENTIRE block is silently ignored

The parser is strict by design. Any one of these and the whole block is
treated as absent: the PR renders with no map, only a discreet notice. There
is no partial acceptance.

- Unquoted bullet text: `- bullet: added stuff` — must be
  `- bullet: "added stuff"`.
- Stray syntax inside the block: comments, tabs, 3-space indents, an unknown
  key, a duplicate `changes:` or top-level key, `lines: 5-2`, `lines: 0`,
  `file: /abs/path`, `file: ../x`, `file: src\x`, an unquoted or invalid
  ignore glob, YAML anchors/aliases/tags (`&x`, `*x`, `!x`).
- Blowing a limit: a 51st bullet, an 11th file entry in one bullet, or a
  block over 4 KB.

### Semantic: the map renders, but wrong

These parse fine — the panel still appears, but it misleads reviewers. Fix
them before requesting review.

- Old-side line numbers taken from the left gutter → those ranges show as
  invalid references (⚠ badge). Always use the new side.
- Lockfiles or generated files missing from `ignore` → they count as unmapped
  and drag coverage down.
- Rewording a description bullet without updating the map → orphan entries:
  they still validate, but the panel shows a warning.

## Keep it fresh

- Update the block after EVERY new commit or force-push, BEFORE requesting
  review. New commits shift line numbers; drifted ranges show up as invalid
  references — that drift detection is the feature, not a bug. Re-read the
  new diff and fix every `lines:` value.
- Then re-run the Done-when gate above.
