#!/usr/bin/env bash
# make-demo.sh -- create the `reviewmap-demo` repository with a realistic
# ~3000-line "Advanced Query" pull request whose description carries a
# review-map block authored exactly per contrib/ai-skills/review-map/SKILL.md,
# plus three control PRs (no block / malformed block / over-limit block) and
# per-user bullet progress for two reviewers.
#
# Requirements: Git Bash on Windows (or any bash), curl, git, sha1sum, awk.
#
# Environment variables:
#   GITEA_URL        base URL of the Gitea instance (default http://127.0.0.1:3000)
#   GITEA_TOKEN      access token of the admin user, scopes: all (required)
#   DEMO_DIR         work directory for the clone and generated files
#                    (default <script dir>/runtime/demo-work)
#   DEMO_OWNER       user that owns the demo repo     (default demo)
#   DEMO_PASS        password of DEMO_OWNER, used for a browser-like cookie
#                    login that drives the progress endpoint and HTML checks
#                    (default DemoAdmin1!)
#   REVIEWER2_USER   second reviewer account          (default reviewer2)
#   REVIEWER2_PASS   its password                     (default DemoReview2!)
#
# The PR body is pr-body.md (bullets only); this script appends the
# ```review-map block with line ranges computed from the diff it just created.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GITEA_URL="${GITEA_URL:-http://127.0.0.1:3000}"
GITEA_TOKEN="${GITEA_TOKEN:?GITEA_TOKEN (admin token, scopes all) must be set}"
DEMO_DIR="${DEMO_DIR:-$SCRIPT_DIR/runtime/demo-work}"
DEMO_OWNER="${DEMO_OWNER:-demo}"
DEMO_PASS="${DEMO_PASS:-DemoAdmin1!}"
REVIEWER2_USER="${REVIEWER2_USER:-reviewer2}"
REVIEWER2_PASS="${REVIEWER2_PASS:-DemoReview2!}"

REPO=reviewmap-demo
BASELINE_LINES=500          # lines of each source file on main
FAILURES=0

say()  { printf '\n== %s\n' "$*"; }
note() { printf '   %s\n' "$*"; }
die()  { printf 'FATAL: %s\n' "$*" >&2; exit 1; }

check() { # check <label> <status>
  if [ "$2" -eq 0 ]; then printf 'PASS  %s\n' "$1"; else printf 'FAIL  %s\n' "$1"; FAILURES=$((FAILURES+1)); fi
}

api() { # api <method> <path> [json-body] -> body on stdout
  local method=$1 path=$2 body=${3:-}
  if [ -n "$body" ]; then
    curl -sS -X "$method" -H "Authorization: token $GITEA_TOKEN" -H 'Content-Type: application/json' -d "$body" "$GITEA_URL/api/v1$path"
  else
    curl -sS -X "$method" -H "Authorization: token $GITEA_TOKEN" "$GITEA_URL/api/v1$path"
  fi
}

json_escape() { # stdin -> JSON string literal body (no surrounding quotes)
  # backslashes and double quotes escaped, newlines -> \n. The input must not
  # end with a newline (the callers pass $BODY-style vars, which never do).
  sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' \
    | awk 'NR > 1 { printf "\\n" } { printf "%s", $0 }'
}

# login <user> <pass> <cookie-jar>: browser-like session. The progress
# endpoint and the HTML pages are web routes; API tokens do not work on them.
login() {
  local user=$1 pass=$2 jar=$3
  rm -f "$jar"
  curl -sS -c "$jar" -o /dev/null "$GITEA_URL/user/login"
  curl -sS -b "$jar" -c "$jar" -o /dev/null \
    -H "Origin: $GITEA_URL" \
    --data-urlencode "user_name=$user" --data-urlencode "password=$pass" \
    "$GITEA_URL/user/login"
  curl -sS -b "$jar" -c "$jar" -o /dev/null -w '%{http_code}' "$GITEA_URL/user/settings" \
    | grep -q '^200$' || die "cookie login failed for $user"
  note "logged in as $user"
}

toggle() { # toggle <jar> <pr-index> <hash> <reviewed>
  local jar=$1 idx=$2 hash=$3 reviewed=$4 code
  code=$(curl -sS -b "$jar" -o /dev/null -w '%{http_code}' \
    -H "Origin: $GITEA_URL" -H 'Content-Type: application/json' \
    -d "{\"hash\":\"$hash\",\"reviewed\":$reviewed}" \
    "$GITEA_URL/$DEMO_OWNER/$REPO/pulls/$idx/review-map/progress")
  [ "$code" = "204" ] || die "progress toggle returned $code (expected 204)"
}

sha1_of() { printf '%s' "$1" | sha1sum | cut -d' ' -f1; }
sorted_pair() { # sorted_pair <a> <b>: echoes the two strings sorted
  if [ "$1" \< "$2" ]; then printf '%s\n%s\n' "$1" "$2"; else printf '%s\n%s\n' "$2" "$1"; fi
}

# ---------------------------------------------------------------------------
# Content generators: placeholder but realistically-shaped.
# emit_lines <count> <Tag> <startIndex> <mode: baseline|advanced|lock|snap>
# Emits exactly <count> lines to stdout (8-line blocks for TS modes, 6 for
# lockfile entries, 4 for snapshot entries, plus single-line fillers).
# ---------------------------------------------------------------------------
emit_lines() {
  local total=$1 tag=$2 i=$3 mode=$4 out=0 left
  while [ "$out" -lt "$total" ]; do
    left=$((total - out))
    case "$mode" in
      advanced)
        if [ "$left" -lt 6 ]; then printf 'export const advancedQueryStage%d = %d; // single-line filler %d\n' "$i" "$i" "$i"; out=$((out+1)); i=$((i+1)); continue; fi
        {
          printf '  /** AdvancedQuery: apply criteria pass %d to the query window. */\n' "$i"
          printf '  export function advancedQueryFold%d(window: QueryWindow%d): FoldResult%d {\n' "$i" "$i" "$i"
          printf '    const criteria = parseAdvancedQuery(window.text, %d);\n' "$i"
          printf "    if (criteria.valid === false) return { ok: false, code: 'AQ_INVALID_%d' };\n" "$i"
          printf '    return { ok: true, rows: window.rows.map((r) => r.score(%d)) };\n' "$i"
          printf '  } // AQ fold %d\n' "$i"
        }
        out=$((out+6)); i=$((i+1))
        ;;
      lock)
        if [ "$left" -lt 6 ]; then printf '    "node_modules/aq-util-%d": { "version": "3.%d.1" },\n' "$i" "$((i % 8))"; out=$((out+1)); i=$((i+1)); continue; fi
        {
          printf '    "node_modules/advanced-query-core-%d": {\n' "$i"
          printf '      "version": "2.%d.0",\n' "$((i % 12))"
          printf '      "resolved": "https://registry.npmjs.org/advanced-query-core-%d/-/advanced-query-core-%d-2.%d.0.tgz",\n' "$i" "$i" "$((i % 12))"
          printf '      "integrity": "sha512-AQcore%s%dWw==",\n' "$((i % 97))" "$i"
          printf '    },\n'
        }
        out=$((out+6)); i=$((i+1))
        ;;
      snap)
        if [ "$left" -lt 4 ]; then printf 'exports[`QueryBuilder empty %d`] = `<div class="qb-empty" data-i="%d" />`;\n' "$i" "$i"; out=$((out+1)); i=$((i+1)); continue; fi
        {
          printf 'exports[`QueryBuilder renders advanced criteria %d`] = `\n' "$i"
          printf '  <div class="qb-row" data-pass="%d">\n' "$i"
          printf '    <span class="qb-criteria">advanced row %d</span>\n' "$i"
          printf '  </div>\n`;\n'
        }
        out=$((out+4)); i=$((i+1))
        ;;
      *) # baseline
        if [ "$left" -lt 8 ]; then printf 'export const %sFlag%d = 1 << %d; // single-line filler %d\n' "$tag" "$i" "$((i % 13))" "$i"; out=$((out+1)); i=$((i+1)); continue; fi
        {
          printf '  /** %s pass %d: normalize and fold the criteria window. */\n' "$tag" "$i"
          printf '  export function %sPass%d(input: %sInput): %sResult {\n' "$tag" "$i" "$tag" "$tag"
          printf '    const criteria = normalize%s(input.criteria, %d);\n' "$tag" "$i"
          printf '    if (!criteria.valid) {\n'
          printf "      return { ok: false, reason: invalid%sReason(%d) };\n" "$tag" "$i"
          printf '    }\n'
          printf '    return fold%sRows(criteria, input.rows.slice(%d %% 7));\n' "$tag" "$((i % 7))"
          printf '  }\n'
        }
        out=$((out+8)); i=$((i+1))
        ;;
    esac
  done
}

# baseline_module <path> <PascalTag>: exactly $BASELINE_LINES lines
baseline_module() {
  local path=$1 tag=$2
  mkdir -p "$(dirname "$path")"
  {
    printf '// %s module (baseline) for the review-map demo repository.\n' "$tag"
    printf '// Placeholder but realistic-shaped TypeScript: state, criteria folding, mappers.\n'
    printf "import { runtimeBus, type Disposable } from '../runtime';\n"
    printf '\n'
    printf 'export interface %sState {\n' "$tag"
    printf '  revision: number;\n'
    printf '  criteriaText: string;\n'
    printf '  rows: readonly %sRow[];\n' "$tag"
    printf "  status: 'idle' | 'loading' | 'ready' | 'failed';\n"
    printf '}\n'
    printf '\n'
  } > "$path"
  emit_lines $((BASELINE_LINES - $(wc -l < "$path"))) "$tag" 1 baseline >> "$path"
}

# replace_lines <path> <first> <oldCount> <newCount> <Tag> <startIdx> <mode>
# Replaces <oldCount> lines starting at <first> with exactly <newCount> new
# lines (del + add in one hunk keeps every hunk line-mappable).
replace_lines() {
  local path=$1 first=$2 old=$3 new=$4 tag=$5 idx=$6 mode=$7 tmp
  tmp="$path.tmp"
  head -n $((first - 1)) "$path" > "$tmp"
  emit_lines "$new" "$tag" "$idx" "$mode" >> "$tmp"
  tail -n +$((first + old)) "$path" >> "$tmp"
  mv "$tmp" "$path"
}

# insert_after <path> <afterLine> <count> <Tag> <startIdx> <mode>
insert_after() {
  local path=$1 after=$2 count=$3 tag=$4 idx=$5 mode=$6 tmp
  tmp="$path.tmp"
  head -n "$after" "$path" > "$tmp"
  emit_lines "$count" "$tag" "$idx" "$mode" >> "$tmp"
  tail -n +$((after + 1)) "$path" >> "$tmp"
  mv "$tmp" "$path"
}

# ---------------------------------------------------------------------------
# The 6 review-map bullets (byte-exact in description AND map block; this
# script is the single source of truth for both, as pr-body.md documents).
# ---------------------------------------------------------------------------
BULLETS=(
  "Added Advanced Query slice and selectors to manage state and criteria"
  "Updated infinite row datasource to support fetching data from the Advanced Query endpoint"
  "Added queryClient API wrapper with batching, retry and typed error handling"
  "Added useAdvancedQuery hook with polling and cache invalidation"
  "Reworked AppState slice to track query lifecycle bookkeeping"
  "Added QueryBuilder component with criteria editing and validation"
)
FILES=(
  src/store/slices/AdvancedQuery.ts
  src/store/selectors/AdvancedQuery.ts
  src/utilities/infiniteRowDatasource.ts
  src/api/queryClient.ts
  src/hooks/useAdvancedQuery.ts
  src/store/slices/AppState.ts
  src/components/QueryBuilder.ts
)
BULLET_FILES=(
  "src/store/slices/AdvancedQuery.ts src/store/selectors/AdvancedQuery.ts"
  "src/utilities/infiniteRowDatasource.ts"
  "src/api/queryClient.ts"
  "src/hooks/useAdvancedQuery.ts"
  "src/store/slices/AppState.ts"
  "src/components/QueryBuilder.ts"
)
PASCAL_TAGS=(AdvancedQuerySlice AdvancedQuerySelectors InfiniteRowDatasource QueryClient UseAdvancedQueryHook AppStateSlice QueryBuilder)

# compute_ranges <file>: one "start-end" cluster per line, new-side numbers
# with 3 lines of context, merged when padding makes clusters touch. Computed
# from the actual `git diff -U0` output -- never by hand.
compute_ranges() {
  local file=$1
  local maxend
  maxend=$(wc -l < "$file")
  git diff -U0 main HEAD -- "$file" | awk -v ctx=3 -v maxend="$maxend" '
    /^@@ / {
      match($0, /\+[0-9]+(,[0-9]+)?/); s = substr($0, RSTART + 1, RLENGTH - 1)
      split(s, a, ","); c = a[1] + 0; d = (a[2] == "" ? 1 : a[2] + 0)
      hunk_end = (d == 0) ? c : c + d - 1
      hunk_start = (d == 0) ? (c > 1 ? c - 1 : 1) : c   # deletion-only hunk: anchor on the preceding new-side line
      start = hunk_start - ctx; if (start < 1) start = 1
      end = hunk_end + ctx; if (end > maxend) end = maxend
      if (n == 0 || start > hi[n] + 1) { n++; lo[n] = start; hi[n] = end }
      else if (end > hi[n]) hi[n] = end
    }
    END { for (j = 1; j <= n; j++) printf "%d-%d\n", lo[j], hi[j] }
  '
}

# ===========================================================================
say "0. Preflight"
command -v curl >/dev/null || die "curl not found"
command -v git >/dev/null || die "git not found"
command -v sha1sum >/dev/null || die "sha1sum not found"
api GET /version | grep -q '"version"' || die "Gitea API not reachable at $GITEA_URL (is the demo instance up?)"
note "Gitea API reachable: $(api GET /version)"

say "1. Users"
code=$(curl -sS -o /tmp/mkdemo-user.json -w '%{http_code}' -X POST \
  -H "Authorization: token $GITEA_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$REVIEWER2_USER\",\"email\":\"$REVIEWER2_USER@demo.local\",\"password\":\"$REVIEWER2_PASS\",\"must_change_password\":false}" \
  "$GITEA_URL/api/v1/admin/users")
case "$code" in
  201) note "created $REVIEWER2_USER" ;;
  422) note "$REVIEWER2_USER already exists" ;;
  *) die "creating $REVIEWER2_USER returned $code: $(cat /tmp/mkdemo-user.json)" ;;
esac

say "2. Repository"
code=$(curl -sS -o /dev/null -w '%{http_code}' -X DELETE \
  -H "Authorization: token $GITEA_TOKEN" "$GITEA_URL/api/v1/repos/$DEMO_OWNER/$REPO")
case "$code" in
  204 | 404) note "old repo gone (http $code)" ;;
  *) die "deleting old repo returned $code" ;;
esac
api POST /user/repos "{\"name\":\"$REPO\",\"private\":false,\"auto_init\":false,\"default_branch\":\"main\",\"description\":\"Demo repo for the review-map feature (AI-generated PR review)\"}" > /tmp/mkdemo-repo.json
grep -q '"full_name"' /tmp/mkdemo-repo.json || die "repo creation failed: $(cat /tmp/mkdemo-repo.json)"
note "created $DEMO_OWNER/$REPO"

# push URL with token as password (git over http)
PUSH_URL="${GITEA_URL/http:\/\//http:\/\/$DEMO_OWNER:$GITEA_TOKEN@}/$DEMO_OWNER/$REPO.git"

say "3. Seed main branch (~$((BASELINE_LINES * 7)) source lines + lockfile + snapshot)"
WORK="$DEMO_DIR/work"
rm -rf "$WORK"
mkdir -p "$WORK"
cd "$WORK"
git init -q -b main .
git config user.name "Demo Author"
git config user.email "demo-author@demo.local"
git config core.autocrlf false

i=0
for f in "${FILES[@]}"; do
  baseline_module "$f" "${PASCAL_TAGS[$i]}"
  i=$((i + 1))
done

# package-lock.json: 150 lines of realistic-shaped lockfile entries
{
  printf '{\n  "name": "reviewmap-demo",\n  "version": "1.0.0",\n  "lockfileVersion": 3,\n  "requires": true,\n  "packages": {\n'
  printf '    "": {\n      "name": "reviewmap-demo",\n      "version": "1.0.0",\n      "dependencies": { "react": "^19.0.0", "redux": "^5.0.0" }\n    },\n'
  n=0
  while [ $n -lt 135 ]; do
    printf '    "node_modules/pkg-%03d": {\n      "version": "1.%d.%d",\n      "resolved": "https://registry.npmjs.org/pkg-%03d/-/pkg-%03d-1.%d.%d.tgz",\n      "integrity": "sha512-%s%d%s"\n    },\n' \
      "$n" "$((n % 9))" "$((n % 17))" "$n" "$n" "$((n % 9))" "$((n % 17))" "LYyFZxTq" "$n" "AbCdEf+/"
    n=$((n + 1))
  done
  printf '    "node_modules/zone.js": { "version": "0.15.0", "dev": true }\n  }\n}\n'
} > package-lock.json

# src/foo.test.snap: 40 lines of snapshot baselines
mkdir -p src
{
  printf '// Snapshot v1, for @vitest/snapshot. For more info: https://vitest.dev/\n\n'
  printf 'exports[`QueryBuilder renders criteria form 1`] = `\n'
  n=0
  while [ $n -lt 30 ]; do
    printf '  <div class="qb-row" data-index="%d">criteria row %d: score %d</div>\n' "$n" "$n" "$((n * 3))"
    n=$((n + 1))
  done
  printf '`;\n\n'
  printf 'exports[`QueryBuilder renders empty state 1`] = `<div class="qb-empty" />`;\n'
} > src/foo.test.snap

printf '# reviewmap-demo\n\nBaseline application used to demo the review-map feature.\nRun `pnpm install && pnpm test` after checking out.\n' > README.md

git add -A
git commit -qm "Initial import: baseline modules, lockfile, snapshots"
git remote add origin "$PUSH_URL"
git push -q "$PUSH_URL" main
note "main pushed: $(git ls-files | wc -l) files, $(git rev-list --count HEAD) commit(s)"

say "4. Feature branch: touch 6 logical areas across ${#FILES[@]} files (~3000 changed lines)"
git checkout -qb feature/advanced-query
# Per source file, 4 mixed/insertion clusters (applied bottom-up so original
# line numbers stay valid):
#   replace lines 60..99   (40 lines)  with 110 advanced lines  -> ~150 changed
#   replace lines 200..229 (30 lines)  with 22 advanced lines   -> ~52 changed
#   insert 190 advanced lines after line 350                    -> 190 changed
#   replace lines 491..500 (10 lines)  with 25 advanced lines   -> ~35 changed
i=0
for f in "${FILES[@]}"; do
  replace_lines "$f" 491 10 25 "${PASCAL_TAGS[$i]}" 300 advanced
  insert_after "$f" 350 190 "${PASCAL_TAGS[$i]}" 400 advanced
  replace_lines "$f" 200 30 22 "${PASCAL_TAGS[$i]}" 500 advanced
  replace_lines "$f" 60 40 110 "${PASCAL_TAGS[$i]}" 600 advanced
  i=$((i + 1))
done
# lockfile: swap 20 middle lines for 100 (new transitive dependencies)
replace_lines package-lock.json 60 20 100 Lockfile 700 lock
# snapshot: 30 new baseline lines after line 20
insert_after src/foo.test.snap 20 30 Snapshots 800 snap

git add -A
git commit -qm "Add Advanced Query feature: store, selectors, API client, datasource, hook, component"

TOTAL_CHANGED=$(git diff --numstat main HEAD | awk '{s += $1 + $2} END {print s}')
SRC_CHANGED=$(git diff --numstat main HEAD -- "${FILES[@]}" | awk '{s += $1 + $2} END {print s}')
note "changed lines: $TOTAL_CHANGED total, $SRC_CHANGED in the ${#FILES[@]} source files"
git push -q "$PUSH_URL" feature/advanced-query

say "5. Compute new-side ranges from the actual diff; author the review-map block"
: > "$DEMO_DIR/ranges.txt"
for f in "${FILES[@]}"; do
  ranges="$(compute_ranges "$f" | tr '\n' ' ' | sed 's/ *$//')"
  [ -n "$ranges" ] || die "no diff hunks computed for $f"
  printf '%s\t%s\n' "$f" "$ranges" >> "$DEMO_DIR/ranges.txt"
  note "$f -> $ranges"
done

MAP_BLOCK=$'ignore:\n  - "package-lock.json"\n  - "src/**/*.snap"\nbullets:'
for b in "${!BULLETS[@]}"; do
  MAP_BLOCK+=$'\n'"  - bullet: \"${BULLETS[$b]}\""$'\n'"    changes:"
  for f in ${BULLET_FILES[$b]}; do
    ranges=$(awk -F'\t' -v f="$f" '$1 == f {print $2}' "$DEMO_DIR/ranges.txt")
    [ -n "$ranges" ] || die "no ranges found for $f"
    for r in $ranges; do
      MAP_BLOCK+=$'\n'"      - file: $f"$'\n'"        lines: $r"
    done
  done
done
BLOCK_BYTES=$(printf '%s' "$MAP_BLOCK" | wc -c)
note "review-map block: $BLOCK_BYTES bytes (instance limit 4096)"
[ "$BLOCK_BYTES" -le 4096 ] || die "generated block exceeds MAX_BLOCK_BYTES"

BODY="$(cat "$SCRIPT_DIR/pr-body.md")
\`\`\`review-map
$MAP_BLOCK
\`\`\`"
printf '%s\n' "$BODY" > "$DEMO_DIR/pr-body-final.md"
note "final PR body written to $DEMO_DIR/pr-body-final.md"

say "6. Open the pull requests"
api POST "/repos/$DEMO_OWNER/$REPO/pulls" \
  "{\"head\":\"feature/advanced-query\",\"base\":\"main\",\"title\":\"Add Advanced Query feature (store, selectors, API client, datasource, hook, component)\",\"body\":\"$(printf '%s' "$BODY" | json_escape)\"}" > /tmp/mkdemo-pr1.json
PR1=$(grep -o '"number":[0-9]*' /tmp/mkdemo-pr1.json | head -1 | cut -d: -f2)
[ -n "$PR1" ] || die "feature PR creation failed: $(cat /tmp/mkdemo-pr1.json)"
printf '%s\n' "$PR1" > "$DEMO_DIR/pr1.index"   # drift.sh reads this
note "feature PR #$PR1: $GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR1"

# --- control PR: tiny change, NO review-map block ----------------------------
git checkout -qb tiny/readme-note main
printf '\n## Running the demo\n\nSee contrib/ai-skills/review-map/demo/README.md in the gitea checkout.\n' >> README.md
git commit -qam "docs: point contributors at the review-map demo runbook"
git push -q "$PUSH_URL" tiny/readme-note
api POST "/repos/$DEMO_OWNER/$REPO/pulls" \
  '{"head":"tiny/readme-note","base":"main","title":"docs: point contributors at the review-map demo runbook","body":"## Summary\n\n- Small documentation pointer; no review map needed for a tiny diff like this.\n"}' > /tmp/mkdemo-pr2.json
PR2=$(grep -o '"number":[0-9]*' /tmp/mkdemo-pr2.json | head -1 | cut -d: -f2)
note "control PR #$PR2 (no block): $GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR2"

# --- control PR: MALFORMED block (unquoted bullet text -> whole block dropped)
git checkout -q main && git checkout -qb bad/malformed-map
printf '\n<!-- malformed-map marker -->\n' >> README.md
git commit -qam "wip: malformed review map"
git push -q "$PUSH_URL" bad/malformed-map
api POST "/repos/$DEMO_OWNER/$REPO/pulls" \
  '{"head":"bad/malformed-map","base":"main","title":"wip: malformed review map","body":"## Summary\n\n- Deliberately malformed map for A5\n\n```review-map\nbullets:\n  - bullet: added something (unquoted on purpose)\n    changes:\n      - file: README.md\n        lines: 1-5\n```\n"}' > /tmp/mkdemo-pr3.json
PR3=$(grep -o '"number":[0-9]*' /tmp/mkdemo-pr3.json | head -1 | cut -d: -f2)
note "control PR #$PR3 (malformed): $GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR3"

# --- control PR: OVER-LIMIT block (> 4096 bytes between the fences) ----------
git checkout -q main && git checkout -qb bad/oversized-map
printf '\n<!-- oversized-map marker -->\n' >> README.md
git commit -qam "wip: oversized review map"
git push -q "$PUSH_URL" bad/oversized-map
OVER_BLOCK="bullets:"
n=0
while [ $n -lt 45 ]; do
  OVER_BLOCK+=$'\n'"  - bullet: \"Filler bullet number $n whose only purpose is to push this review-map block past the four kilobyte instance limit for testing purposes\""$'\n'"    changes:"$'\n'"      - file: README.md"$'\n'"        lines: $((n + 1))-$((n + 9))"
  n=$((n + 1))
done
OVER_BODY_TEXT="## Summary

- Deliberately over-limit map for A5

\`\`\`review-map
$OVER_BLOCK
\`\`\`"
api POST "/repos/$DEMO_OWNER/$REPO/pulls" \
  "{\"head\":\"bad/oversized-map\",\"base\":\"main\",\"title\":\"wip: oversized review map\",\"body\":\"$(printf '%s' "$OVER_BODY_TEXT" | json_escape)\"}" > /tmp/mkdemo-pr4.json
PR4=$(grep -o '"number":[0-9]*' /tmp/mkdemo-pr4.json | head -1 | cut -d: -f2)
note "control PR #$PR4 (over-limit): $GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR4"

say "7. Files in the feature PR (API check)"
api GET "/repos/$DEMO_OWNER/$REPO/pulls/$PR1/files?limit=50" > "$DEMO_DIR/pr1-files.json"
FILES_IN_PR=$(grep -o '"filename"' "$DEMO_DIR/pr1-files.json" | wc -l)
note "$FILES_IN_PR files in the diff (expect 9: 7 sources + package-lock.json + src/foo.test.snap)"
[ "$FILES_IN_PR" -eq 9 ] || die "expected 9 files in the feature PR diff, got $FILES_IN_PR"

say "8. Two reviewers toggle bullets (A4)"
login "$DEMO_OWNER" "$DEMO_PASS" "$DEMO_DIR/admin.jar"
login "$REVIEWER2_USER" "$REVIEWER2_PASS" "$DEMO_DIR/reviewer2.jar"
H0=$(sha1_of "${BULLETS[0]}"); H1=$(sha1_of "${BULLETS[1]}")
H2=$(sha1_of "${BULLETS[2]}"); H3=$(sha1_of "${BULLETS[3]}")
toggle "$DEMO_DIR/admin.jar" "$PR1" "$H0" true
toggle "$DEMO_DIR/admin.jar" "$PR1" "$H1" true
toggle "$DEMO_DIR/reviewer2.jar" "$PR1" "$H2" true
toggle "$DEMO_DIR/reviewer2.jar" "$PR1" "$H3" true
note "$DEMO_OWNER reviewed bullets 1-2, $REVIEWER2_USER reviewed bullets 3-4"
readarray -t SORT01 < <(sorted_pair "$H0" "$H1")
readarray -t SORT23 < <(sorted_pair "$H2" "$H3")

say "9. HTML verification (A2/A4/A5/A8)"
curl -sS -b "$DEMO_DIR/admin.jar" "$GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR1" > "$DEMO_DIR/pr1.html"
curl -sS -b "$DEMO_DIR/admin.jar" "$GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR1/files" > "$DEMO_DIR/pr1-files.html"
curl -sS -b "$DEMO_DIR/admin.jar" "$GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR2" > "$DEMO_DIR/pr2.html"
curl -sS -b "$DEMO_DIR/admin.jar" "$GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR3" > "$DEMO_DIR/pr3.html"
curl -sS -b "$DEMO_DIR/admin.jar" "$GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR4" > "$DEMO_DIR/pr4.html"
curl -sS -b "$DEMO_DIR/reviewer2.jar" "$GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR1" > "$DEMO_DIR/pr1-rev2.html"

st=0; grep -q 'id="review-map-panel"' "$DEMO_DIR/pr1.html" || st=1
check "A2 conversation page renders #review-map-panel" "$st"
st=0; grep -q 'tw-bg-green' "$DEMO_DIR/pr1.html" || st=1; grep -q 'Verified' "$DEMO_DIR/pr1.html" || st=1
check "A2 badge reads Verified" "$st"
st=0; grep -qE "0 of $SRC_CHANGED changed lines unmapped" "$DEMO_DIR/pr1.html" || st=1
check "A2/A8 coverage line: 0 of $SRC_CHANGED changed lines unmapped" "$st"
for b in "${BULLETS[@]}"; do
  st=0; grep -qF "$b" "$DEMO_DIR/pr1.html" || st=1
  check "A2 page contains bullet: $b" "$st"
done
st=0; grep -q '"invalidRefCount":0' "$DEMO_DIR/pr1.html" || st=1
check "pageData invalidRefCount = 0" "$st"
st=0; grep -q '"orphanCount":0' "$DEMO_DIR/pr1.html" || st=1
check "pageData orphanCount = 0" "$st"
st=0; grep -qE "\"TotalChanged\":$SRC_CHANGED,\"MappedChanged\":$SRC_CHANGED" "$DEMO_DIR/pr1.html" || st=1
check "A8 pageData coverage MappedChanged = TotalChanged = $SRC_CHANGED" "$st"
st=0; grep -q '"UnmappedFiles":null' "$DEMO_DIR/pr1.html" || st=1
check "A8 pageData UnmappedFiles empty" "$st"
st=0; grep -qF 'package-lock.json' "$DEMO_DIR/pr1.html" || st=1; grep -qF 'src/**/*.snap' "$DEMO_DIR/pr1.html" || st=1
check "A8 panel lists both ignore globs" "$st"
st=0; grep -q 'id="diff-file-boxes"' "$DEMO_DIR/pr1-files.html" || st=1
check "files page has #diff-file-boxes" "$st"
st=0; grep -q 'id="review-map-panel"' "$DEMO_DIR/pr1-files.html" || st=1
check "files page renders the panel" "$st"
st=0; grep -qF 'package-lock.json' "$DEMO_DIR/pr1-files.html" || st=1
check "A8 lockfile still visible in Files Changed" "$st"
BOXES=$(grep -o 'diff-file-box ' "$DEMO_DIR/pr1-files.html" | wc -l)
st=0; [ "$BOXES" -ge 9 ] || st=1
check "files page shows all 9 diff file boxes (found $BOXES)" "$st"
st=0; grep -qF "$DEMO_OWNER 2/6" "$DEMO_DIR/pr1.html" || st=1
check "A4 progress row: $DEMO_OWNER 2/6" "$st"
st=0; grep -qF "$REVIEWER2_USER 2/6" "$DEMO_DIR/pr1.html" || st=1
check "A4 progress row: $REVIEWER2_USER 2/6" "$st"
st=0; grep -q "\"myReviewed\":\[\"${SORT01[0]}\",\"${SORT01[1]}\"\]" "$DEMO_DIR/pr1.html" || st=1
check "A4 admin view shows own reviewed bullets (persisted)" "$st"
st=0; grep -q "\"myReviewed\":\[\"${SORT23[0]}\",\"${SORT23[1]}\"\]" "$DEMO_DIR/pr1-rev2.html" || st=1
check "A4 reviewer2 view shows own reviewed bullets (per-user)" "$st"
# A5: control PRs render upstream-style
st=0; grep -q 'review-map-panel' "$DEMO_DIR/pr2.html" && st=1; grep -q 'review-map block ignored' "$DEMO_DIR/pr2.html" && st=1
check "A5 PR#$PR2 (no block): no panel, no warning" "$st"
st=0; grep -q 'review-map-panel' "$DEMO_DIR/pr3.html" && st=1
grep -q 'review-map block ignored: malformed' "$DEMO_DIR/pr3.html" || st=1
check "A5 PR#$PR3 (malformed): no panel, discreet warning" "$st"
st=0; grep -q 'review-map-panel' "$DEMO_DIR/pr4.html" && st=1
grep -q 'review-map block ignored: too_large' "$DEMO_DIR/pr4.html" || st=1
check "A5 PR#$PR4 (over-limit): no panel, discreet warning" "$st"

say "Done. Failures: $FAILURES"
printf 'PR URLs:\n  feature:    %s/%s/%s/pulls/%s\n  no block:   %s/%s/%s/pulls/%s\n  malformed:  %s/%s/%s/pulls/%s\n  over-limit: %s/%s/%s/pulls/%s\n' \
  "$GITEA_URL" "$DEMO_OWNER" "$REPO" "$PR1" \
  "$GITEA_URL" "$DEMO_OWNER" "$REPO" "$PR2" \
  "$GITEA_URL" "$DEMO_OWNER" "$REPO" "$PR3" \
  "$GITEA_URL" "$DEMO_OWNER" "$REPO" "$PR4"
[ "$FAILURES" -eq 0 ] || die "$FAILURES verification(s) failed -- inspect $DEMO_DIR/*.html"
exit 0
