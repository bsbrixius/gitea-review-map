// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package reviewmap

import (
	"strconv"
	"strings"
	"testing"

	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	reviewmapmod "gitea.dev/modules/reviewmap"
	"gitea.dev/modules/setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pagedataIssue() *issues_model.Issue {
	return &issues_model.Issue{
		IsPull:  true,
		Index:   4,
		Repo:    &repo_model.Repository{OwnerName: "owner", Name: "repo"},
		Content: "- bullet a\n",
	}
}

func pagedataResult() *Result {
	return &Result{
		Bullets: []BulletStatus{
			bulletStatus(0, "bullet a", false),
			bulletStatus(1, "bullet b", false),
			bulletStatus(2, "bullet c", true),
		},
		InvalidRefCount: 1,
		OrphanCount:     1,
		Coverage:        Coverage{TotalChanged: 10, MappedChanged: 8, UnmappedFiles: []string{"pnpm-lock.yaml"}},
	}
}

func TestBuildPageData(t *testing.T) {
	res := pagedataResult()
	rm := &reviewmapmod.Map{Ignore: []string{"pnpm-lock.yaml"}}
	users := map[int64]*user_model.User{
		1: {ID: 1, Name: "alice"},
		2: {ID: 2, Name: "bob"},
	}
	// alice reviewed two real bullets plus a hash that matches no bullet;
	// bob reviewed one. Only hashes present in the result may count as Done.
	progress := map[int64]map[string]bool{
		1: {res.Bullets[0].Hash: true, res.Bullets[1].Hash: true, HashBullet("not in result"): true},
		2: {res.Bullets[0].Hash: true},
	}
	comments := map[string][]CommentLink{
		res.Bullets[0].Hash: {
			{ID: 11, URL: "/owner/repo/pulls/4/files#issuecomment-11"},
			{ID: 12, URL: "/owner/repo/pulls/4/files#issuecomment-12"},
		},
	}

	pd := BuildPageData(pagedataIssue(), res, rm, progress, users, comments)

	assert.Empty(t, pd.IgnoredReason)
	assert.False(t, pd.CanReview)
	assert.Equal(t, "/owner/repo/pulls/4", pd.IssueLink)
	assert.Equal(t, "/owner/repo/pulls/4/review-map/progress", pd.ToggleURL)
	assert.Equal(t, res.Bullets, pd.Bullets)
	assert.Equal(t, comments, pd.CommentLinks)
	assert.Equal(t, res.Coverage, pd.Coverage)
	assert.Equal(t, []string{"pnpm-lock.yaml"}, pd.IgnoreGlobs)
	assert.Equal(t, 1, pd.InvalidRefCount)
	assert.Equal(t, 1, pd.OrphanCount)

	// ordered by Done desc: alice (2) before bob (1); Total is the bullet count
	require.Len(t, pd.Progress, 2)
	assert.Equal(t, UserProgress{UserID: 1, Name: "alice", Done: 2, Total: 3}, pd.Progress[0])
	assert.Equal(t, UserProgress{UserID: 2, Name: "bob", Done: 1, Total: 3}, pd.Progress[1])
}

func TestBuildPageDataProgressOrdering(t *testing.T) {
	res := pagedataResult()
	users := map[int64]*user_model.User{
		1: {ID: 1, Name: "zoe"},
		2: {ID: 2, Name: "alice"},
		3: {ID: 3, Name: "bob"},
		4: {ID: 4, Name: "ghost"}, // no progress entry: must not appear
	}
	progress := map[int64]map[string]bool{
		1: {res.Bullets[0].Hash: true},
		2: {res.Bullets[0].Hash: true},
		3: {res.Bullets[0].Hash: true, res.Bullets[1].Hash: true},
		9: {res.Bullets[0].Hash: true}, // user missing from users map: must not appear
	}

	pd := BuildPageData(pagedataIssue(), res, nil, progress, users, nil)

	// Done desc first (bob=2), then Name asc among equal Done (alice, zoe)
	require.Len(t, pd.Progress, 3)
	assert.Equal(t, []int64{3, 2, 1}, []int64{pd.Progress[0].UserID, pd.Progress[1].UserID, pd.Progress[2].UserID})
	assert.Equal(t, []string{"bob", "alice", "zoe"}, []string{pd.Progress[0].Name, pd.Progress[1].Name, pd.Progress[2].Name})
	assert.Empty(t, pd.IgnoreGlobs) // nil map is tolerated
	assert.Nil(t, pd.CommentLinks)
}

func TestPreparePageDataIgnoredReason(t *testing.T) {
	saved := setting.ReviewMap
	defer func() { setting.ReviewMap = saved }()

	// a block well under the default cap, used to exercise the pre-I/O paths
	smallBlock := "```review-map\nbullets:\n  - bullet: \"a bullet\"\n```\n"

	tests := []struct {
		name         string
		enabled      bool
		isPull       bool
		maxBlockByte int
		content      string
		want         *PageData
	}{
		{name: "feature disabled", enabled: false, isPull: true, content: smallBlock, want: nil},
		{name: "not a pull request", enabled: true, isPull: false, content: smallBlock, want: nil},
		{name: "no block", enabled: true, isPull: true, content: "just a description", want: nil},
		{name: "unclosed block", enabled: true, isPull: true, content: "```review-map\nbullets:\n", want: nil},
		{
			name: "block too large", enabled: true, isPull: true, maxBlockByte: 10,
			content: smallBlock,
			want:    &PageData{IgnoredReason: IgnoredReasonTooLarge},
		},
		{
			name: "block malformed", enabled: true, isPull: true,
			content: "```review-map\nbullets:\n  bogus\n```\n",
			want:    &PageData{IgnoredReason: IgnoredReasonMalformed},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setting.ReviewMap.Enabled = tc.enabled
			setting.ReviewMap.MaxBullets = 50
			setting.ReviewMap.MaxFilesPerBullet = 10
			setting.ReviewMap.MaxBlockBytes = 4 * 1024
			if tc.maxBlockByte != 0 {
				setting.ReviewMap.MaxBlockBytes = tc.maxBlockByte
			}
			issue := pagedataIssue()
			issue.IsPull = tc.isPull
			issue.Content = tc.content
			assert.Equal(t, tc.want, PreparePageData(t.Context(), nil, issue))
		})
	}

	// sanity: the small block's byte length really sits between the two caps used above
	block, status := reviewmapmod.ExtractBlock(smallBlock, 4*1024)
	require.Equal(t, reviewmapmod.BlockOK, status)
	assert.Greater(t, len(block), 10)
	assert.True(t, strings.Contains(block, "bullet"))
}

func TestMyReviewedHashes(t *testing.T) {
	// the MyReviewed field is assigned in PreparePageData from the progress map; its
	// logic lives in this pure helper so it can be covered without I/O
	progress := map[int64]map[string]bool{
		1: {"hash-b": true, "hash-a": true, "hash-off": false},
		2: {"hash-z": true},
	}
	assert.Equal(t, []string{"hash-a", "hash-b"}, myReviewedHashes(progress, 1))
	assert.Empty(t, myReviewedHashes(progress, 9)) // user without progress
}

func refProgressKey(hash string, i int) string { return hash + ":i" + strconv.Itoa(i) }

func refApprovalResult() *Result {
	res := pagedataResult()
	res.Bullets[0].Refs = []RefStatus{
		refStatus("a.ts", 1, 2, true),
		refStatus("b.ts", 1, 2, true),
		refStatus("bad.ts", 1, 2, false), // invalid refs never count toward completion
	}
	return res
}

func TestBuildPageDataRefApprovalDerivation(t *testing.T) {
	res := refApprovalResult()
	b0 := res.Bullets[0].Hash
	users := map[int64]*user_model.User{
		1: {ID: 1, Name: "both-refs"},   // approves both valid refs, no bullet hash
		2: {ID: 2, Name: "one-ref"},     // partial: not done
		3: {ID: 3, Name: "whole"},       // bullet hash alone
		4: {ID: 4, Name: "invalid-ref"}, // only the invalid ref's key
	}
	progress := map[int64]map[string]bool{
		1: {refProgressKey(b0, 0): true, refProgressKey(b0, 1): true},
		2: {refProgressKey(b0, 0): true},
		3: {b0: true},
		4: {refProgressKey(b0, 2): true},
	}
	pd := BuildPageData(pagedataIssue(), res, nil, progress, users, nil)
	doneByUser := map[int64]int{}
	for _, p := range pd.Progress {
		doneByUser[p.UserID] = p.Done
	}
	assert.Equal(t, 1, doneByUser[1], "all valid refs approved completes the bullet")
	assert.Equal(t, 0, doneByUser[2], "partial ref approval does not complete the bullet")
	assert.Equal(t, 1, doneByUser[3], "bullet-level hash still completes the bullet")
	assert.Equal(t, 0, doneByUser[4], "approving an invalid ref completes nothing")
}

func TestBuildBulletApproval(t *testing.T) {
	res := refApprovalResult()
	b0 := res.Bullets[0].Hash
	keys := map[string]bool{refProgressKey(b0, 0): true} // one of two valid refs
	approval := BuildBulletApproval(res.Bullets, keys)
	require.Len(t, approval, len(res.Bullets))
	assert.Equal(t, ApprovalCount{Approved: 1, Total: 2}, approval[0], "invalid refs are not counted")
	assert.Equal(t, ApprovalCount{Approved: 0, Total: 0}, approval[1])

	keys[b0] = true // whole bullet: per-ref counters stay as they are
	approval = BuildBulletApproval(res.Bullets, keys)
	assert.Equal(t, ApprovalCount{Approved: 1, Total: 2}, approval[0])

	assert.Empty(t, BuildBulletApproval(res.Bullets, nil))
}
