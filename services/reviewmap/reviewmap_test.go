// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package reviewmap

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"testing"

	"gitea.dev/modules/git"
	reviewmapmod "gitea.dev/modules/reviewmap"
	"gitea.dev/services/gitdiff"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashBullet(t *testing.T) {
	text := "Added Advanced Query slice and selectors to manage state and criteria"
	sum := sha1.Sum([]byte(text))
	assert.Equal(t, hex.EncodeToString(sum[:]), HashBullet(text))
	assert.Equal(t, HashBullet("same text"), HashBullet("same text"))
	assert.NotEqual(t, HashBullet("bullet one"), HashBullet("bullet two"))
	assert.Len(t, HashBullet(""), 40)
}

func refStatus(file string, start, end int, valid bool) RefStatus {
	return RefStatus{File: file, NameHash: git.HashFilePathForWebUI(file), StartLine: start, EndLine: end, Valid: valid}
}

func bulletStatus(index int, text string, orphan bool, refs ...RefStatus) BulletStatus {
	return BulletStatus{Index: index, Hash: HashBullet(text), Text: text, Orphan: orphan, Refs: refs}
}

func TestComputeResult(t *testing.T) {
	appTsPatch := `diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -1,2 +1,3 @@
 a
+b
 c
`
	contextPatch := `diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -10,5 +10,5 @@
 l10
 l11
-old12
+new12
 l13
 l14
`

	type testcase struct {
		name        string
		nilDiff     bool
		patch       string
		m           *reviewmapmod.Map
		descBullets []string
		want        *Result
	}

	tests := []testcase{
		{
			name: "spec example: 8 of 8 mapped, ignored lockfile and snapshot",
			patch: `diff --git a/src/store/slices/AdvancedQuery.ts b/src/store/slices/AdvancedQuery.ts
--- a/src/store/slices/AdvancedQuery.ts
+++ b/src/store/slices/AdvancedQuery.ts
@@ -1,3 +1,6 @@
 line1
 line2
+add1
+add2
+add3
 line4
diff --git a/src/store/selectors/AdvancedQuery.ts b/src/store/selectors/AdvancedQuery.ts
--- a/src/store/selectors/AdvancedQuery.ts
+++ b/src/store/selectors/AdvancedQuery.ts
@@ -1,3 +1,5 @@
 sel1
+newSelA
+newSelB
 sel2
 sel3
diff --git a/src/utilities/infiniteRowDatasource.ts b/src/utilities/infiniteRowDatasource.ts
--- a/src/utilities/infiniteRowDatasource.ts
+++ b/src/utilities/infiniteRowDatasource.ts
@@ -30,4 +30,5 @@
 ctxA
-oldA
+newA
 ctxB
+newC
 ctxC
diff --git a/package-lock.json b/package-lock.json
--- a/package-lock.json
+++ b/package-lock.json
@@ -1,3 +1,4 @@
 {
   "x": 1
+  "new": 1
 }
diff --git a/src/snapshots/ui.snap b/src/snapshots/ui.snap
--- a/src/snapshots/ui.snap
+++ b/src/snapshots/ui.snap
@@ -1,3 +1,3 @@
 snapshot 1
-snapshot 2
+snapshot 2 updated
 snapshot 3
`,
			m: &reviewmapmod.Map{
				Ignore: []string{"package-lock.json", "src/**/*.snap"},
				Bullets: []reviewmapmod.Bullet{
					{Text: "Added Advanced Query slice and selectors to manage state and criteria", Changes: []reviewmapmod.FileRange{
						{File: "src/store/slices/AdvancedQuery.ts", StartLine: 1, EndLine: 120},
						{File: "src/store/selectors/AdvancedQuery.ts", StartLine: 1, EndLine: 45},
					}},
					{Text: "Updated infinite row datasource to support fetching data from the Advanced Query endpoint", Changes: []reviewmapmod.FileRange{
						{File: "src/utilities/infiniteRowDatasource.ts", StartLine: 30, EndLine: 88},
					}},
				},
			},
			descBullets: []string{
				"Added Advanced Query slice and selectors to manage state and criteria",
				"Updated infinite row datasource to support fetching data from the Advanced Query endpoint",
			},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Added Advanced Query slice and selectors to manage state and criteria", false,
						refStatus("src/store/slices/AdvancedQuery.ts", 1, 120, true),
						refStatus("src/store/selectors/AdvancedQuery.ts", 1, 45, true),
					),
					bulletStatus(1, "Updated infinite row datasource to support fetching data from the Advanced Query endpoint", false,
						refStatus("src/utilities/infiniteRowDatasource.ts", 30, 88, true),
					),
				},
				Coverage:    Coverage{TotalChanged: 8, MappedChanged: 8},
				IgnoreGlobs: []string{"package-lock.json", "src/**/*.snap"},
			},
		},
		{
			name:  "range on context lines only: valid but maps 0 of 2",
			patch: contextPatch,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Tweak app", Changes: []reviewmapmod.FileRange{{File: "app.ts", StartLine: 13, EndLine: 14}}},
			}},
			descBullets: []string{"Tweak app"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Tweak app", false, refStatus("app.ts", 13, 14, true)),
				},
				Coverage: Coverage{TotalChanged: 2, MappedChanged: 0, UnmappedFiles: []string{"app.ts"}},
			},
		},
		{
			name:  "range outside every hunk: invalid",
			patch: contextPatch,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Tweak app", Changes: []reviewmapmod.FileRange{{File: "app.ts", StartLine: 50, EndLine: 60}}},
			}},
			descBullets: []string{"Tweak app"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Tweak app", false, refStatus("app.ts", 50, 60, false)),
				},
				InvalidRefCount: 1,
				Coverage:        Coverage{TotalChanged: 2, MappedChanged: 0, UnmappedFiles: []string{"app.ts"}},
			},
		},
		{
			name:  "range on absent file: invalid",
			patch: contextPatch,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Tweak app", Changes: []reviewmapmod.FileRange{{File: "missing.ts", StartLine: 1, EndLine: 5}}},
			}},
			descBullets: []string{"Tweak app"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Tweak app", false, refStatus("missing.ts", 1, 5, false)),
				},
				InvalidRefCount: 1,
				Coverage:        Coverage{TotalChanged: 2, MappedChanged: 0, UnmappedFiles: []string{"app.ts"}},
			},
		},
		{
			name: "deletion anchoring: 2 distinct positions cover 4 changed lines",
			patch: `diff --git a/anchor.txt b/anchor.txt
--- a/anchor.txt
+++ b/anchor.txt
@@ -10,6 +10,4 @@
 ctx10
-delA
-delB
 ctx11
+add11
-delC
 ctx12
`,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Rework anchor", Changes: []reviewmapmod.FileRange{{File: "anchor.txt", StartLine: 10, EndLine: 12}}},
			}},
			descBullets: []string{"Rework anchor"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Rework anchor", false, refStatus("anchor.txt", 10, 12, true)),
				},
				Coverage: Coverage{TotalChanged: 4, MappedChanged: 2},
			},
		},
		{
			name: "deletion before any new line: anchor 0, valid range still maps nothing",
			patch: `diff --git a/head.txt b/head.txt
--- a/head.txt
+++ b/head.txt
@@ -1,2 +1,1 @@
-delFirst
 ctx1
`,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Drop first line", Changes: []reviewmapmod.FileRange{{File: "head.txt", StartLine: 1, EndLine: 1}}},
			}},
			descBullets: []string{"Drop first line"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Drop first line", false, refStatus("head.txt", 1, 1, true)),
				},
				Coverage: Coverage{TotalChanged: 1, MappedChanged: 0, UnmappedFiles: []string{"head.txt"}},
			},
		},
		{
			name: "fully deleted file: deletions unmapped, no new-side hunk, ref invalid",
			patch: `diff --git a/gone.txt b/gone.txt
deleted file mode 100644
index 0123456..789abcd 100644
--- a/gone.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-bye1
-bye2
`,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Remove gone", Changes: []reviewmapmod.FileRange{{File: "gone.txt", StartLine: 1, EndLine: 2}}},
			}},
			descBullets: []string{"Remove gone"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Remove gone", false, refStatus("gone.txt", 1, 2, false)),
				},
				InvalidRefCount: 1,
				Coverage:        Coverage{TotalChanged: 2, MappedChanged: 0, UnmappedFiles: []string{"gone.txt"}},
			},
		},
		{
			name: "binary listed without totals, submodule skipped entirely",
			// quoted head names: unquoted ones keep a trailing space from readFileName's
			// space-splitting heuristic (binary files have no +++ line to fix the name up)
			patch: `diff --git "a/logo.png" "b/logo.png"
index 0123456..789abcd 100644
Binary files a/logo.png and b/logo.png differ
diff --git a/vendor/lib b/vendor/lib
new file mode 160000
index 0000000..1234567
--- /dev/null
+++ b/vendor/lib
@@ -0,0 +1 @@
+Subproject commit 1234567890abcdef1234567890abcdef12345678
diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -1,2 +1,3 @@
 a
+b
 c
`,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Add logo", Changes: []reviewmapmod.FileRange{{File: "app.ts", StartLine: 1, EndLine: 3}}},
			}},
			descBullets: []string{"Add logo"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Add logo", false, refStatus("app.ts", 1, 3, true)),
				},
				Coverage: Coverage{TotalChanged: 1, MappedChanged: 1, UnmappedFiles: []string{"logo.png"}},
			},
		},
		{
			name: "ignore glob removes file from totals and validation",
			patch: `diff --git a/src/ui.ts b/src/ui.ts
--- a/src/ui.ts
+++ b/src/ui.ts
@@ -1,2 +1,3 @@
 a
+b
 c
diff --git a/src/snapshots/case1.snap b/src/snapshots/case1.snap
--- a/src/snapshots/case1.snap
+++ b/src/snapshots/case1.snap
@@ -1,2 +1,2 @@
-old
+new
 tail
`,
			m: &reviewmapmod.Map{
				Ignore: []string{"src/**/*.snap"},
				Bullets: []reviewmapmod.Bullet{
					{Text: "Update UI and snapshot", Changes: []reviewmapmod.FileRange{
						{File: "src/ui.ts", StartLine: 1, EndLine: 3},
						{File: "src/snapshots/case1.snap", StartLine: 1, EndLine: 2},
					}},
				},
			},
			descBullets: []string{"Update UI and snapshot"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Update UI and snapshot", false,
						refStatus("src/ui.ts", 1, 3, true),
						refStatus("src/snapshots/case1.snap", 1, 2, false),
					),
				},
				InvalidRefCount: 1,
				Coverage:        Coverage{TotalChanged: 1, MappedChanged: 1},
				IgnoreGlobs:     []string{"src/**/*.snap"},
			},
		},
		{
			name: "ignore glob with ** matches direct children too (gitignore style)",
			patch: `diff --git a/src/ui.ts b/src/ui.ts
--- a/src/ui.ts
+++ b/src/ui.ts
@@ -1,2 +1,3 @@
 a
+b
 c
diff --git a/src/foo.test.snap b/src/foo.test.snap
--- a/src/foo.test.snap
+++ b/src/foo.test.snap
@@ -1,2 +1,2 @@
-old
+new
 tail
`,
			m: &reviewmapmod.Map{
				Ignore: []string{"src/**/*.snap"},
				Bullets: []reviewmapmod.Bullet{
					{Text: "Update UI", Changes: []reviewmapmod.FileRange{{File: "src/ui.ts", StartLine: 1, EndLine: 3}}},
				},
			},
			descBullets: []string{"Update UI"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Update UI", false, refStatus("src/ui.ts", 1, 3, true)),
				},
				Coverage:    Coverage{TotalChanged: 1, MappedChanged: 1},
				IgnoreGlobs: []string{"src/**/*.snap"},
			},
		},
		{
			name:  "two bullets mapping the same line count it once",
			patch: appTsPatch,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "First change", Changes: []reviewmapmod.FileRange{{File: "app.ts", StartLine: 1, EndLine: 2}}},
				{Text: "Second change", Changes: []reviewmapmod.FileRange{{File: "app.ts", StartLine: 2, EndLine: 3}}},
			}},
			descBullets: []string{"First change", "Second change"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "First change", false, refStatus("app.ts", 1, 2, true)),
					bulletStatus(1, "Second change", false, refStatus("app.ts", 2, 3, true)),
				},
				Coverage: Coverage{TotalChanged: 1, MappedChanged: 1},
			},
		},
		{
			name: "file referenced by no bullet: all its lines unmapped and listed",
			patch: `diff --git a/mapped.ts b/mapped.ts
--- a/mapped.ts
+++ b/mapped.ts
@@ -1,2 +1,3 @@
 a
+b
 c
diff --git a/other.ts b/other.ts
--- a/other.ts
+++ b/other.ts
@@ -1,3 +1,4 @@
 x
+y1
+y2
 z
-w
`,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Map one file", Changes: []reviewmapmod.FileRange{{File: "mapped.ts", StartLine: 1, EndLine: 3}}},
			}},
			descBullets: []string{"Map one file"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Map one file", false, refStatus("mapped.ts", 1, 3, true)),
				},
				Coverage: Coverage{TotalChanged: 4, MappedChanged: 1, UnmappedFiles: []string{"other.ts"}},
			},
		},
		{
			name: "incomplete file excluded from totals and unmapped list",
			patch: "diff --git a/big.ts b/big.ts\n--- a/big.ts\n+++ b/big.ts\n@@ -1,1 +1,3 @@\n keep\n+" +
				strings.Repeat("x", 5000) + "\n+short\ndiff --git a/app.ts b/app.ts\n--- a/app.ts\n+++ b/app.ts\n@@ -1,2 +1,3 @@\n a\n+b\n c\n",
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Keep app tidy", Changes: []reviewmapmod.FileRange{{File: "app.ts", StartLine: 1, EndLine: 3}}},
			}},
			descBullets: []string{"Keep app tidy"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Keep app tidy", false, refStatus("app.ts", 1, 3, true)),
				},
				Coverage: Coverage{TotalChanged: 1, MappedChanged: 1},
			},
		},
		{
			name:  "orphan bullet flagged but still validates and maps",
			patch: appTsPatch,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Current text", Changes: []reviewmapmod.FileRange{{File: "app.ts", StartLine: 1, EndLine: 3}}},
				{Text: "Stale text", Changes: []reviewmapmod.FileRange{{File: "app.ts", StartLine: 1, EndLine: 3}}},
			}},
			descBullets: []string{"Current text", "Unrelated bullet"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Current text", false, refStatus("app.ts", 1, 3, true)),
					bulletStatus(1, "Stale text", true, refStatus("app.ts", 1, 3, true)),
				},
				OrphanCount: 1,
				Coverage:    Coverage{TotalChanged: 1, MappedChanged: 1},
			},
		},
		{
			name:    "nil diff: every ref invalid, zero coverage",
			nilDiff: true,
			m: &reviewmapmod.Map{Bullets: []reviewmapmod.Bullet{
				{Text: "Current text", Changes: []reviewmapmod.FileRange{{File: "app.ts", StartLine: 1, EndLine: 3}}},
			}},
			descBullets: []string{"Current text"},
			want: &Result{
				Bullets: []BulletStatus{
					bulletStatus(0, "Current text", false, refStatus("app.ts", 1, 3, false)),
				},
				InvalidRefCount: 1,
			},
		},
		{
			name:  "nil map: changed file still counted and listed",
			patch: appTsPatch,
			m:     nil,
			want: &Result{
				Bullets:  []BulletStatus{},
				Coverage: Coverage{TotalChanged: 1, MappedChanged: 0, UnmappedFiles: []string{"app.ts"}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// maxLineCharacters must stay positive: ParsePatch slices line[:maxLineCharacters]
			// whenever len(line) > limit, so -1 panics; 5000 is the upstream default.
			diff, err := gitdiff.ParsePatch(t.Context(), -1, 5000, -1, strings.NewReader(tc.patch), "")
			require.NoError(t, err)
			if tc.nilDiff {
				diff = nil
			}
			assert.Equal(t, tc.want, ComputeResult(tc.m, diff, tc.descBullets))
		})
	}
}
