import fs from 'node:fs';
let s = fs.readFileSync('make-demo.sh', 'utf8');
const rep = (oldStr, newStr) => {
  if (!s.includes(oldStr)) { console.error('ANCHOR MISSING: ' + JSON.stringify(oldStr.slice(0, 70))); process.exit(1); }
  s = s.replace(oldStr, () => newStr);
};
const section = (startMark, endMark, body) => {
  const a = s.indexOf(startMark), b = s.indexOf(endMark);
  if (a === -1 || b === -1 || b <= a) { console.error('section markers bad'); process.exit(1); }
  s = s.slice(0, a) + body + s.slice(b);
};

section('# ---------------------------------------------------------------------------\n# Content generators:',
        '# ---------------------------------------------------------------------------\n# The 6 review-map bullets', '');

rep(`REPO=reviewmap-demo
BASELINE_LINES=500          # lines of each source file on main
`, () => `REPO=reviewmap-demo
`);

rep(`BULLETS=(
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
PASCAL_TAGS=(AdvancedQuerySlice AdvancedQuerySelectors InfiniteRowDatasource QueryClient UseAdvancedQueryHook AppStateSlice QueryBuilder)`,
() => `BULLETS=(
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
)`);

section('say "3. Seed main branch', 'say "4. Feature branch', `say "3. Seed main branch from fixtures/main (real pra-quando-api sources)"
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

`);

section('say "4. Feature branch', 'say "5. Compute new-side ranges', `say "4. Feature branch: overlay fixtures/feature (surgical changes across the source files)"
git checkout -qb feature/result-combinators
cp -r "$SCRIPT_DIR/fixtures/feature/." .

git add -A
git commit -qm "Add Result combinators and harden availability queries"

TOTAL_CHANGED=$(git diff --numstat main HEAD | awk '{s += $1 + $2} END {print s}')
SRC_CHANGED=$(git diff --numstat main HEAD -- src test | awk '{s += $1 + $2} END {print s}')
note "changed lines: $TOTAL_CHANGED total, $SRC_CHANGED in the source files (Directory.Packages.props is ignored)"
git push -q "$PUSH_URL" feature/result-combinators

`);

rep(`MAP_BLOCK=$'ignore:\n  - "package-lock.json"\n  - "src/**/*.snap"\nbullets:'`,
    () => `MAP_BLOCK=$'ignore:\n  - "Directory.Packages.props"\nbullets:'`);
rep(`"{\"head\":\"feature/advanced-query\",\"base\":\"main\",\"title\":\"Add Advanced Query feature (store, selectors, API client, datasource, hook, component)\"`,
    () => `"{\"head\":\"feature/result-combinators\",\"base\":\"main\",\"title\":\"Add Result combinators and harden availability queries\"`);

rep(`note "$FILES_IN_PR files in the diff (expect 9: 7 sources + package-lock.json + src/foo.test.snap)"
[ "$FILES_IN_PR" -eq 9 ] || die "expected 9 files in the feature PR diff, got $FILES_IN_PR"`,
() => `note "$FILES_IN_PR files in the diff (expect 8: 7 sources + Directory.Packages.props)"
[ "$FILES_IN_PR" -eq 8 ] || die "expected 8 files in the feature PR diff, got $FILES_IN_PR"`);

rep(`st=0; grep -qF 'package-lock.json' "$DEMO_DIR/pr1.html" || st=1; grep -qF 'src/**/*.snap' "$DEMO_DIR/pr1.html" || st=1
check "A8 panel lists both ignore globs" "$st"`,
() => `st=0; grep -qF 'Directory.Packages.props' "$DEMO_DIR/pr1.html" || st=1
check "A8 panel lists the ignore glob" "$st"`);

rep(`st=0; grep -qF 'package-lock.json' "$DEMO_DIR/pr1-files.html" || st=1
check "A8 lockfile still visible in Files Changed" "$st"
BOXES=$(grep -o 'diff-file-box ' "$DEMO_DIR/pr1-files.html" | wc -l)
st=0; [ "$BOXES" -ge 9 ] || st=1
check "files page shows all 9 diff file boxes (found $BOXES)" "$st"`,
() => `st=0; grep -qF 'Directory.Packages.props' "$DEMO_DIR/pr1-files.html" || st=1
check "A8 package-props file still visible in Files Changed" "$st"
BOXES=$(grep -o 'diff-file-box ' "$DEMO_DIR/pr1-files.html" | wc -l)
st=0; [ "$BOXES" -ge 8 ] || st=1
check "files page shows all 8 diff file boxes (found $BOXES)" "$st"`);

rep(`# make-demo.sh -- create the \`reviewmap-demo\` repository with a realistic
# ~3000-line "Advanced Query" pull request whose description carries a
# review-map block authored exactly per contrib/ai-skills/review-map/SKILL.md,`,
() => `# make-demo.sh -- create the \`reviewmap-demo\` repository with a realistic
# "Result combinators + availability hardening" pull request over real C#
# sources (fixtures/ holds before/after trees from the pra-quando-api
# codebase: surgical changes, new methods, no whole-file rewrites). The
# description carries a review-map block authored exactly per
# contrib/ai-skills/review-map/SKILL.md,`);

fs.writeFileSync('make-demo.sh', s);
console.log('reworked:', s.split('\n').length, 'lines');
