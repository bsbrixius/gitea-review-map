# Review Map demo: runbook and acceptance walkthrough

Everything needed to build the fork, run a local instance, and walk the
review-map acceptance criteria A1–A8 end to end. Commands are copy-pasteable
for Git Bash on Windows with Docker Desktop running. `cd` into the `gitea`
checkout first; all paths below are relative to it.

- Demo scripts: `contrib/ai-skills/review-map/demo/make-demo.sh`, `drift.sh`
- PR body template: `contrib/ai-skills/review-map/demo/pr-body.md`
- Runtime state (git-ignored): `contrib/ai-skills/review-map/demo/runtime/`
- Instance: Docker container **`gitea-demo`** on http://127.0.0.1:3000

## 1. Build (A1)

The host has node but no Go/make, so the frontend builds on the host and the
backend builds in a `golang:1.27` container. Two named Docker volumes
(`gitea-gomod`, `gitea-gobuild`) cache modules and build cache between runs.

Frontend (host; `node_modules` must exist — `corepack pnpm install --frozen-lockfile`):

```bash
corepack pnpm exec vite build     # writes public/assets/.vite/manifest.json
```

Backend (container; CGO-free, sqlite via modernc is the default driver, no
`bindata` tag: templates/public/options are served from the mounted checkout,
so no `go generate` step is needed):

```bash
MSYS_NO_PATHCONV=1 docker run --rm \
  -v 'C:\Development\Gitea-AI\gitea:/repo' \
  -v gitea-gomod:/go/pkg/mod -v gitea-gobuild:/root/.cache/go-build \
  -w /repo -e CGO_ENABLED=0 golang:1.27 go build -o gitea .
```

(The equivalent upstream targets are `make frontend` + `make backend`; the
commands above replicate them without host Go/make.)

## 2. Configure and run the instance (A1)

Write `contrib/ai-skills/review-map/demo/runtime/app.ini` (directory is
git-ignored; generate a fresh `SECRET_KEY` with `openssl rand -hex 32`):

```ini
RUN_USER = root
I_AM_BEING_UNSAFE_RUNNING_AS_ROOT = true
WORK_PATH = /repo/contrib/ai-skills/review-map/demo/runtime

[server]
PROTOCOL = http
HTTP_ADDR = 0.0.0.0
HTTP_PORT = 3000
ROOT_URL = http://127.0.0.1:3000/
DOMAIN = 127.0.0.1
STATIC_ROOT_PATH = /repo
DISABLE_SSH = true
SSH_PORT = 22
OFFLINE_MODE = true
LFS_START_SERVER = false
ENABLE_PPROF = false

[database]
DB_TYPE = sqlite3
PATH = /repo/contrib/ai-skills/review-map/demo/runtime/data/gitea.db
LOG_SQL = false

[security]
INSTALL_LOCK = true
SECRET_KEY = <openssl rand -hex 32>
LOGIN_REMEMBER_DAYS = 7
PASSWORD_COMPLEXITY = lower,upper,digit

[repository]
ROOT = /repo/contrib/ai-skills/review-map/demo/runtime/data/git/repositories
DEFAULT_BRANCH = main
DISABLE_HTTP_GIT = false

[git]
MAX_GIT_DIFF_LINES = 10000    ; so the ~3000-line demo PR renders fully
MAX_GIT_DIFF_FILES = 1000

[review_map]
; instance defaults, listed for documentation
ENABLED = true
MAX_BULLETS = 50
MAX_FILES_PER_BULLET = 10
MAX_BLOCK_BYTES = 4096

[session]
PROVIDER = file
PROVIDER_CONFIG = /repo/contrib/ai-skills/review-map/demo/runtime/data/sessions
COOKIE_SECURE = false

[service]
DISABLE_REGISTRATION = true
REQUIRE_SIGNIN_VIEW = false
DEFAULT_KEEP_EMAIL_PRIVATE = true
DEFAULT_ALLOW_CREATE_ORGANIZATION = false
ENABLE_NOTIFY_MAIL = false

[mailer]
ENABLED = false

[oauth2]
ENABLED = false

[actions]
ENABLED = false

[packages]
ENABLED = false

[mirror]
ENABLED = false

[log]
MODE = console
LEVEL = Info
ENABLE_SSH_LOG = false

[ui]
DEFAULT_THEME = gitea-auto
THEMES = gitea-auto,gitea-light,gitea-dark
```

Start (leaves a running container named `gitea-demo`):

```bash
MSYS_NO_PATHCONV=1 docker run -d --name gitea-demo -p 3000:3000 \
  -v 'C:\Development\Gitea-AI\gitea:/repo' -w /repo \
  golang:1.27 /repo/gitea web \
  --config /repo/contrib/ai-skills/review-map/demo/runtime/app.ini
```

Stop / restart / logs:

```bash
docker stop gitea-demo      # stop (docker start gitea-demo to bring it back)
docker restart gitea-demo   # restart (progress and data persist in sqlite)
docker logs -f gitea-demo
```

## 3. Admin bootstrap and API token (A1)

Passwords are never hardcoded: export your own values first, they are used
verbatim by the commands below and by both scripts.

```bash
export DEMO_PASS='<your-admin-password>'        # admin user "demo"
export REVIEWER2_PASS='<your-reviewer2-password>'
```

```bash
# admin user (idempotent: a second call fails with "user already exists")
MSYS_NO_PATHCONV=1 docker exec -w /repo gitea-demo ./gitea admin user create \
  --admin --username demo --password "$DEMO_PASS" --email demo@demo.local \
  --must-change-password=false \
  --config /repo/contrib/ai-skills/review-map/demo/runtime/app.ini

# API token (scopes: all) -- copy the "sha1" value
curl -sS -u "demo:$DEMO_PASS" -H 'Content-Type: application/json' \
  -d '{"name":"demo-token","scopes":["all"]}' \
  http://127.0.0.1:3000/api/v1/users/demo/tokens
```

Second reviewer account (also created by `make-demo.sh` automatically; on
re-runs `REVIEWER2_PASS` must match the existing account's password):

```bash
curl -sS -X POST -H "Authorization: token $GITEA_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"username\":\"reviewer2\",\"email\":\"reviewer2@demo.local\",\"password\":\"$REVIEWER2_PASS\",\"must_change_password\":false}" \
  http://127.0.0.1:3000/api/v1/admin/users
```

## 4. Seed the demo repo (A2/A4/A5/A7/A8)

Requires `GITEA_TOKEN`, `DEMO_PASS` and `REVIEWER2_PASS` (see step 3).

```bash
cd contrib/ai-skills/review-map/demo
GITEA_TOKEN=<sha1-from-step-3> bash make-demo.sh   # DEMO_PASS/REVIEWER2_PASS from env
```

What it does (idempotent — it deletes and recreates the repo):

- creates repo `demo/reviewmap-demo` and seeds `main` with 7 TypeScript
  modules of 500 lines each, a ~960-line `package-lock.json`,
  `src/foo.test.snap` and a README;
- branches `feature/advanced-query` touching 6 logical areas across the 7
  files: **≈3000 source changed lines** (the exact total is computed and
  printed by the script at runtime; the lockfile and snapshot changes are
  extra);
- computes new-side line ranges from the actual `git diff -U0` output and
  appends the ```review-map block to `pr-body.md`, producing
  `runtime/demo-work/pr-body-final.md` (the block maps 6 bullets to 32 file
  ranges and ignores `package-lock.json` + `src/**/*.snap` — authored exactly
  per `../SKILL.md`, which is what A7 requires);
- opens 4 PRs:
  - [#1](http://127.0.0.1:3000/demo/reviewmap-demo/pulls/1) the ~3000-line
    feature PR with the review map,
  - [#2](http://127.0.0.1:3000/demo/reviewmap-demo/pulls/2) tiny PR, **no block**,
  - [#3](http://127.0.0.1:3000/demo/reviewmap-demo/pulls/3) **malformed** block
    (unquoted bullet),
  - [#4](http://127.0.0.1:3000/demo/reviewmap-demo/pulls/4) **over-limit** block
    (> 4096 bytes);
- logs in as `demo` and `reviewer2` (cookie sessions, like a browser: the
  progress endpoint is a web route and does not accept API tokens) and marks
  bullets 1–2 / 3–4 reviewed via `POST /demo/reviewmap-demo/pulls/1/review-map/progress`;
- greps the rendered HTML and prints PASS/FAIL for every checkable criterion.

Expected output: `== Done. Failures: 0` with 25 PASS lines.

## 5. Drift (A6)

```bash
cd contrib/ai-skills/review-map/demo
GITEA_TOKEN=<sha1> DEMO_PASS=$DEMO_PASS bash drift.sh   # repeatable; each run shifts another +30
```

Pushes one commit inserting 30 lines at the top of
`src/store/slices/AdvancedQuery.ts`. Every new-side number below shifts by 30,
so the no-longer-updated ranges `267-294` and `740-767` stop intersecting
their hunks: the panel badge flips from **Verified** to **2 invalid
reference(s)** and coverage drops below 100% (the 30 new lines are unmapped).
The two `src/store/slices/AdvancedQuery.ts:267-294` / `:740-767` links render
red with an alert icon. Re-running `make-demo.sh` resets the PR to the clean
state.

## 6. Acceptance checklist (A1–A8)

| # | Criterion | How to check |
|---|-----------|--------------|
| A1 | fork builds & runs locally, commands documented | Sections 1–3 above; `curl -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/` → `200` |
| A2 | ~3000-line PR with a review map shows panel + Verified badge + coverage | Open PR #1. Panel "Review Map" above the description, green **Verified** label, "0 of N changed lines unmapped" (N = the source total the script printed; 2989 in the reference run) |
| A3 | clicking a bullet filters the diff to its mapped lines | Open PR #1, click a bullet in the panel → jumps to Files Changed with only that bullet's file boxes/rows visible (highlighted); "Show all files" restores. Cross-page: on the conversation page the click stores `review-map-filter` in sessionStorage and the files page applies it once |
| A4 | per-user progress, checkboxes persist | Panel shows `demo 2/6` and `reviewer2 2/6`. Log in as `demo` / `reviewer2` (the exported `DEMO_PASS` / `REVIEWER2_PASS`): each sees only own checkboxes ticked; state survives `docker restart gitea-demo` (sqlite-backed) |
| A5 | PRs without a block (and malformed / over-limit) render upstream-style | PR #2: no panel, no warning. PR #3/#4: no panel, only the discreet grey note "review-map block ignored: malformed / too_large" |
| A6 | a line-shifting commit flips the badge to ⚠ invalid | `bash drift.sh`, then look at PR #1: red "2 invalid reference(s)", red file:lines links, coverage < 100% |
| A7 | SKILL.md-authored PR yields a valid map | The demo PR's body (`runtime/demo-work/pr-body-final.md`) is authored strictly per `../SKILL.md` — bullets byte-exact, new-side ranges from the real diff, lockfile+snapshots ignored — and the panel reads Verified with 0 invalid / 0 orphans, which is the SKILL's self-check |
| A8 | lockfile covered by an `ignore` glob: 100% coverage, still visible, globs listed | PR #1 panel: "0 of N changed lines unmapped" (N = the script's source total; lockfile/snapshot excluded), `package-lock.json` + `src/**/*.snap` chips under "Ignored globs", and `package-lock.json` is still a file box on the Files Changed tab |

Manual A4 walkthrough in a browser: log in as `reviewer2`, open PR #1, tick two
bullets, reload — they stay ticked; the panel shows `reviewer2 2/6` for
everyone. Log out, log in as `demo`: different checkboxes ticked, both users'
rows visible.

## Notes

- **Renumber on upstream rebase**: this fork carries one DB migration of its
  own, v343 "Add review map progress table"
  (`models/migrations/v1_27/v343.go`). Upstream will eventually ship its own
  v343, so when rebasing this fork onto a newer upstream `main`, renumber the
  fork's entry and file to the next free migration ID after upstream's list
  (the migrator requires strictly sequential IDs; see the comment at the entry
  in `models/migrations/migrations.go`).
- The scripts are Git Bash + curl + git only (plus `sha1sum`/`awk` shipped
  with Git Bash). `make-demo.sh` re-runs cleanly at any time.
- Cookie jars (`runtime/demo-work/*.jar`) are browser-equivalent sessions;
  they exist because web routes (progress toggle, HTML pages) do not accept
  API tokens.
- `runtime/` holds app.ini, the sqlite DB, git repositories, generated PR body
  and HTML snapshots — delete it and redo sections 2–4 for a factory reset.
- No credentials are stored in this repository: `DEMO_PASS` /
  `REVIEWER2_PASS` come from your environment (step 3) and any credentials a
  running instance uses live only in the git-ignored `runtime/` directory.
