#!/usr/bin/env bash
# drift.sh -- acceptance A6: push a new commit to the feature branch that
# inserts 30 lines at the top of a mapped file. Every new-side line number in
# that file shifts down by 30, so the review-map block (not updated) now
# contains references that no longer intersect their hunks: the panel badge
# must flip from "Verified" to "N invalid reference(s)".
#
# Same environment variables as make-demo.sh:
#   GITEA_URL (default http://127.0.0.1:3000), GITEA_TOKEN (required),
#   DEMO_DIR (default <script dir>/runtime/demo-work),
#   DEMO_OWNER (default demo), DEMO_PASS (required, never hardcoded)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GITEA_URL="${GITEA_URL:-http://127.0.0.1:3000}"
GITEA_TOKEN="${GITEA_TOKEN:?GITEA_TOKEN (admin token, scopes all) must be set}"
DEMO_DIR="${DEMO_DIR:-$SCRIPT_DIR/runtime/demo-work}"
DEMO_OWNER="${DEMO_OWNER:-demo}"
: "${DEMO_PASS:?DEMO_PASS (password of $DEMO_OWNER) must be set in the environment}"

REPO=reviewmap-demo
FILE=src/store/slices/AdvancedQuery.ts   # mapped by bullet 1 of the feature PR
FAILURES=0

say()  { printf '\n== %s\n' "$*"; }
note() { printf '   %s\n' "$*"; }
die()  { printf 'FATAL: %s\n' "$*" >&2; exit 1; }
check() {
  if [ "$2" -eq 0 ]; then printf 'PASS  %s\n' "$1"; else printf 'FAIL  %s\n' "$1"; FAILURES=$((FAILURES+1)); fi
}

say "1. Locate the feature PR"
[ -f "$DEMO_DIR/pr1.index" ] || die "$DEMO_DIR/pr1.index missing -- run make-demo.sh first"
PR=$(cat "$DEMO_DIR/pr1.index")
note "feature PR is #$PR"

say "2. Push a commit that shifts lines (+30 at the top of $FILE)"
# repeatable: every run pushes another +30-line commit (shifts grow); make-demo.sh resets the PR
cd "$DEMO_DIR/work"
PUSH_URL="${GITEA_URL/http:\/\//http:\/\/$DEMO_OWNER:$GITEA_TOKEN@}/$DEMO_OWNER/$REPO.git"
git fetch -q "$PUSH_URL" feature/advanced-query
git checkout -qfB feature/advanced-query FETCH_HEAD
# insert 30 lines at the very top: all following new-side numbers shift by +30
{
  n=0
  while [ $n -lt 30 ]; do
    printf '// AdvancedQuery design note %d/30: the criteria window is folded lazily; see review-map SKILL.\n' "$n"
    n=$((n + 1))
  done
  cat "$FILE"
} > "$FILE.new"
mv "$FILE.new" "$FILE"
git commit -qam "Drift demo: expand module header documentation (+30 lines)"
git push -q "$PUSH_URL" feature/advanced-query
note "pushed $(git rev-parse --short HEAD)"

say "3. Verify the panel flipped to invalid (A6)"
JAR="$DEMO_DIR/admin.jar"
rm -f "$JAR"
curl -sS -c "$JAR" -o /dev/null "$GITEA_URL/user/login"
curl -sS -b "$JAR" -c "$JAR" -o /dev/null -H "Origin: $GITEA_URL" \
  --data-urlencode "user_name=$DEMO_OWNER" --data-urlencode "password=$DEMO_PASS" \
  "$GITEA_URL/user/login"
curl -sS -b "$JAR" "$GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR" > "$DEMO_DIR/pr1-drift.html"

st=0; grep -qE '"invalidRefCount":[1-9]' "$DEMO_DIR/pr1-drift.html" || st=1
check "A6 pageData invalidRefCount > 0" "$st"
st=0; grep -qE 'tw-bg-red' "$DEMO_DIR/pr1-drift.html" || st=1
grep -qE '[1-9][0-9]* invalid reference\(s\)' "$DEMO_DIR/pr1-drift.html" || st=1
check "A6 badge shows N invalid reference(s)" "$st"
st=0; grep -qE '[1-9][0-9]* of [0-9]+ changed lines unmapped' "$DEMO_DIR/pr1-drift.html" || st=1
check "A6 coverage drops below 100% (30 unmapped lines)" "$st"
st=0; grep -q 'tw-bg-green' "$DEMO_DIR/pr1-drift.html" && st=1
check "A6 verified badge gone" "$st"

say "Manual observation for the runbook (A6):"
note "open $GITEA_URL/$DEMO_OWNER/$REPO/pulls/$PR -- the panel badge is now"
note "a red label reading \"N invalid reference(s)\" instead of \"Verified\", and the"
note "affected file:lines links in bullet 1 are rendered as invalid (red + alert icon)."

say "Done. Failures: $FAILURES"
[ "$FAILURES" -eq 0 ] || die "$FAILURES verification(s) failed -- inspect $DEMO_DIR/pr1-drift.html"
exit 0
