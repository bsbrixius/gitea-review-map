#!/usr/bin/env bash
# make-demo.sh -- create the `reviewmap-demo` repository with a realistic
# "Result combinators + availability hardening" pull request over real C#
# sources (fixtures/ holds before/after trees from the pra-quando-api
# codebase: surgical changes, new methods, no whole-file rewrites). The
# description carries a review-map block authored exactly per
# contrib/ai-skills/review-map/SKILL.md,
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
#   DEMO_PASS        password of DEMO_OWNER (required): used for a browser-like
#                    cookie login that drives the progress endpoint and HTML
#                    checks. Never hardcoded -- export your own value.
#   REVIEWER2_USER   second reviewer account          (default reviewer2)
#   REVIEWER2_PASS   its password (required; created by this script, or must
#                    match the existing account's password on re-runs)
#
# The PR body is pr-body.md (bullets only); this script appends the
# ```review-map block with line ranges computed from the diff it just created.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GITEA_URL="${GITEA_URL:-http://127.0.0.1:3000}"
GITEA_TOKEN="${GITEA_TOKEN:?GITEA_TOKEN (admin token, scopes all) must be set}"
DEMO_DIR="${DEMO_DIR:-$SCRIPT_DIR/runtime/demo-work}"
DEMO_OWNER="${DEMO_OWNER:-demo}"
: "${DEMO_PASS:?DEMO_PASS (password of $DEMO_OWNER) must be set in the environment}"
REVIEWER2_USER="${REVIEWER2_USER:-reviewer2}"
: "${REVIEWER2_PASS:?REVIEWER2_PASS (password of $REVIEWER2_USER) must be set in the environment}"

REPO=reviewmap-demo
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
# The 6 review-map bullets (byte-exact in description AND map block; this
# script is the single source of truth for both, as pr-body.md documents).
# ---------------------------------------------------------------------------
BULLETS=(
  "Added Map, Bind and Match combinators to Result for fluent error flows"
  "Introduced availability error codes for past-date windows and inactive categories"
  "Extended IRoomAvailabilityQuery with CountActiveRoomsAsync and implemented it in Catalog"
  "Hardened AvailabilityService against past-date queries with richer no-schedule warnings"
  "Added OperatingHoursDto.Covers for open-window checks"
  "Added unit tests for the new Result combinators"
)
FILES=(
  src/Common/PraQuando.SharedKernel.Domain/Result.cs
  src/Common/PraQuando.SharedKernel.Domain/Errors/ErrorCodes.cs
  src/Common/PraQuando.SharedKernel.Application/CatalogReads/IRoomAvailabilityQuery.cs
  src/Modules/Catalog/PraQuando.Modules.Catalog/Infrastructure/RoomAvailabilityQuery.cs
  src/Common/PraQuando.SharedKernel.Application/CatalogReads/OperatingHoursDto.cs
  src/Modules/Booking/PraQuando.Modules.Booking/Shared/AvailabilityService.cs
  test/PraQuando.UnitTests/SharedKernel/ResultCombinatorTests.cs
)
BULLET_FILES=(
  "src/Common/PraQuando.SharedKernel.Domain/Result.cs"
  "src/Common/PraQuando.SharedKernel.Domain/Errors/ErrorCodes.cs"
  "src/Common/PraQuando.SharedKernel.Application/CatalogReads/IRoomAvailabilityQuery.cs src/Modules/Catalog/PraQuando.Modules.Catalog/Infrastructure/RoomAvailabilityQuery.cs"
  "src/Modules/Booking/PraQuando.Modules.Booking/Shared/AvailabilityService.cs"
  "src/Common/PraQuando.SharedKernel.Application/CatalogReads/OperatingHoursDto.cs"
  "test/PraQuando.UnitTests/SharedKernel/ResultCombinatorTests.cs"
)
# plain-english layer: per-change summaries, per-file notes ("index:file"), details for change 0
SUMMARIES=(
  "Composes error flows without if/else ladders: failures short-circuit and keep the original error."
  "Two stable error codes let API clients tell a past window from an inactive category without parsing messages."
  "Counts rooms in SQL so callers distinguish no-rooms from no-schedule without loading the list."
  "Past-date requests fail fast with a typed error, and the no-schedule warning says how many active rooms exist."
  "One obvious place for open/closed checks, closing time excluded like the slot grid."
  "Six facts pin the combinators, including the short-circuit path."
)
declare -A NOTES=(
  ["2:src/Common/PraQuando.SharedKernel.Application/CatalogReads/IRoomAvailabilityQuery.cs"]="New contract method; its doc says when to prefer it over GetAvailableSlotsAsync."
  ["2:src/Modules/Catalog/PraQuando.Modules.Catalog/Infrastructure/RoomAvailabilityQuery.cs"]="EF CountAsync over the same active-room filter instead of materializing rows."
  ["3:src/Modules/Booking/PraQuando.Modules.Booking/Shared/AvailabilityService.cs"]="Guard first, then the enriched warning: cheap count before the empty-result return."
  ["4:src/Common/PraQuando.SharedKernel.Application/CatalogReads/OperatingHoursDto.cs"]="Record method on the read model: no entity behavior crosses the module boundary."
)
DETAILS_0=(
  "Map and Bind live on Result<TValue> because they need the value; Match lives on the base Result."
  "All three are null-safe: a failure never touches the projection or continuation."
)

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
  422) note "$REVIEWER2_USER already exists (REVIEWER2_PASS must match its password or the login below fails)" ;;
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

say "3. Seed main branch from fixtures/main (real pra-quando-api sources)"
WORK="$DEMO_DIR/work"
rm -rf "$WORK"
mkdir -p "$WORK"
cd "$WORK"
git init -q -b main .
git config user.name "Demo Author"
git config user.email "demo-author@demo.local"
git config core.autocrlf false
cp -r "$SCRIPT_DIR/fixtures/main/." .
printf '# reviewmap-demo\n\nBaseline C# sources imported from the pra-quando-api codebase\nfor the review-map feature demo.\n' > README.md

git add -A
git commit -qm "Initial import: pra-quando-api baseline sources"
git remote add origin "$PUSH_URL"
git push -q "$PUSH_URL" main
note "main pushed: $(git ls-files | wc -l) files, $(git rev-list --count HEAD) commit(s)"

say "4. Feature branch: overlay fixtures/feature (surgical changes across the source files)"
git checkout -qb feature/result-combinators
cp -r "$SCRIPT_DIR/fixtures/feature/." .

git add -A
git commit -qm "Add Result combinators and harden availability queries"

TOTAL_CHANGED=$(git diff --numstat main HEAD | awk '{s += $1 + $2} END {print s}')
SRC_CHANGED=$(git diff --numstat main HEAD -- src test | awk '{s += $1 + $2} END {print s}')
note "changed lines: $TOTAL_CHANGED total, $SRC_CHANGED in the source files (Directory.Packages.props is ignored)"
git push -q "$PUSH_URL" feature/result-combinators

say "5. Compute new-side ranges from the actual diff; author the review-map block"
: > "$DEMO_DIR/ranges.txt"
for f in "${FILES[@]}"; do
  ranges="$(compute_ranges "$f" | tr '\n' ' ' | sed 's/ *$//')"
  [ -n "$ranges" ] || die "no diff hunks computed for $f"
  printf '%s\t%s\n' "$f" "$ranges" >> "$DEMO_DIR/ranges.txt"
  note "$f -> $ranges"
done

MAP_BLOCK=$'ignore:\n  - "Directory.Packages.props"\nbullets:'
for b in "${!BULLETS[@]}"; do
  MAP_BLOCK+=$'\n'"  - bullet: \"${BULLETS[$b]}\""
  [ -n "${SUMMARIES[$b]}" ] && MAP_BLOCK+=$'\n'"    summary: \"${SUMMARIES[$b]}\""
  if [ "$b" = "0" ]; then
    MAP_BLOCK+=$'\n'"    details:"
    for d in "${DETAILS_0[@]}"; do
      MAP_BLOCK+=$'\n'"      - \"$d\""
    done
  fi
  MAP_BLOCK+=$'\n'"    changes:"
  for f in ${BULLET_FILES[$b]}; do
    ranges=$(awk -F'\t' -v f="$f" '$1 == f {print $2}' "$DEMO_DIR/ranges.txt")
    [ -n "$ranges" ] || die "no ranges found for $f"
    for r in $ranges; do
      MAP_BLOCK+=$'\n'"      - file: $f"$'\n'"        lines: $r"
      [ -n "${NOTES[$b:$f]-}" ] && MAP_BLOCK+=$'\n'"        note: \"${NOTES[$b:$f]}\""
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
  "{\"head\":\"feature/result-combinators\",\"base\":\"main\",\"title\":\"Add Result combinators and harden availability queries\",\"body\":\"$(printf '%s' "$BODY" | json_escape)\"}" > /tmp/mkdemo-pr1.json
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
note "$FILES_IN_PR files in the diff (expect 8: 7 sources + Directory.Packages.props)"
[ "$FILES_IN_PR" -eq 8 ] || die "expected 8 files in the feature PR diff, got $FILES_IN_PR"

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
st=0; grep -qF 'Directory.Packages.props' "$DEMO_DIR/pr1.html" || st=1
check "A8 panel lists the ignore glob" "$st"
st=0; grep -q 'id="diff-file-boxes"' "$DEMO_DIR/pr1-files.html" || st=1
check "files page has #diff-file-boxes" "$st"
st=0; grep -q 'id="review-map-panel"' "$DEMO_DIR/pr1-files.html" || st=1
check "files page renders the panel" "$st"
st=0; grep -qF 'Directory.Packages.props' "$DEMO_DIR/pr1-files.html" || st=1
check "A8 package-props file still visible in Files Changed" "$st"
BOXES=$(grep -o 'diff-file-box ' "$DEMO_DIR/pr1-files.html" | wc -l)
st=0; [ "$BOXES" -ge 8 ] || st=1
check "files page shows all 8 diff file boxes (found $BOXES)" "$st"
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
