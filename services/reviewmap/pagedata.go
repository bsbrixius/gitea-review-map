// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package reviewmap

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	issues_model "gitea.dev/models/issues"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/log"
	reviewmapmod "gitea.dev/modules/reviewmap"
	"gitea.dev/modules/setting"
	"gitea.dev/services/gitdiff"
)

const (
	IgnoredReasonTooLarge  = "too_large" // the review-map block exceeds MaxBlockBytes
	IgnoredReasonMalformed = "malformed" // the review-map block violates the restricted subset
)

type CommentLink struct {
	ID  int64  `json:"id"`
	URL string `json:"url"` // issue.Link + "/files#issuecomment-{id}"
}

type UserProgress struct {
	UserID int64  `json:"userId"`
	Name   string `json:"name"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
}

// ApprovalCount is one user's per-ref approval state of one bullet: the sidebar's "k/N approved".
type ApprovalCount struct {
	Approved int `json:"approved"`
	Total    int `json:"total"` // valid refs only; invalid refs never block completion
}

// RefProgressKey is the per-ref progress key: "<bullet hash>:i<ref index>". It reuses the
// Reviewed map of ReviewMapProgress and stays under its 64-char key cap.
func RefProgressKey(bulletHash string, refIndex int) string {
	return fmt.Sprintf("%s:i%d", bulletHash, refIndex)
}

// FlagProgressKey is the per-reviewer discussion-flag key for a whole bullet: "<hash>:flag".
// Like ref keys it lives in the Reviewed map without any schema change.
func FlagProgressKey(bulletHash string) string {
	return bulletHash + ":flag"
}

// PageData drives both the pull-request template (Go fields) and ctx.PageData (json tags).
type PageData struct {
	IgnoredReason   string                   `json:"ignoredReason"` // non-empty: render the warning chip only
	IssueLink       string                   `json:"issueLink"`
	ToggleURL       string                   `json:"toggleUrl"`
	CanReview       bool                     `json:"canReview"`
	Bullets         []BulletStatus           `json:"bullets"`
	CommentLinks    map[string][]CommentLink `json:"commentLinks"` // bullet Hash → links
	Coverage        Coverage                 `json:"coverage"`
	IgnoreGlobs     []string                 `json:"ignoreGlobs"`
	InvalidRefCount int                      `json:"invalidRefCount"`
	OrphanCount     int                      `json:"orphanCount"`
	Progress        []UserProgress           `json:"progress"`
	MyReviewed      []string                 `json:"myReviewed"`     // hashes the signed-in user has reviewed; empty when anonymous
	MyApprovedRefs  []string                 `json:"myApprovedRefs"` // ref progress keys ("<hash>:i<N>") the signed-in user approved
	BulletApproval  []ApprovalCount          `json:"bulletApproval"` // per-bullet k/N of the signed-in user, parallel to Bullets
	MyFlagged       []string                 `json:"myFlagged"`      // bullet hashes the signed-in user flagged for discussion
	BulletFlags     []int                    `json:"bulletFlags"`    // per-bullet flag count across ALL reviewers, parallel to Bullets
	FlaggedCount    int                      `json:"flaggedCount"`   // total flags across reviewers and bullets
}

// BuildPageData assembles the page data from a computed result plus the loaded
// progress, users and comment links. Pure: no I/O.
func BuildPageData(issue *issues_model.Issue, res *Result, rm *reviewmapmod.Map,
	progress map[int64]map[string]bool, users map[int64]*user_model.User,
	comments map[string][]CommentLink,
) *PageData {
	bulletHashes := make(map[string]bool, len(res.Bullets))
	for _, b := range res.Bullets {
		bulletHashes[b.Hash] = true
	}

	progressList := make([]UserProgress, 0, len(progress))
	for userID, hashes := range progress {
		u := users[userID]
		if u == nil {
			continue
		}
		done := 0
		for hash := range hashes {
			if bulletHashes[hash] { // only hashes of bullets in the result count
				done++
			}
		}
		for _, b := range res.Bullets { // completing every valid ref also completes the bullet
			if !hashes[b.Hash] && bulletCompleted(b, hashes) {
				done++
			}
		}
		progressList = append(progressList, UserProgress{UserID: userID, Name: u.DisplayName(), Done: done, Total: len(res.Bullets)})
	}
	slices.SortFunc(progressList, func(a, b UserProgress) int {
		return cmp.Or(cmp.Compare(b.Done, a.Done), cmp.Compare(a.Name, b.Name))
	})

	// discussion flags: counted per bullet across ALL reviewers (everyone sees ⚑ N)
	bulletFlags := make([]int, len(res.Bullets))
	flaggedCount := 0
	for _, hashes := range progress {
		for bi, b := range res.Bullets {
			if hashes[FlagProgressKey(b.Hash)] {
				bulletFlags[bi]++
				flaggedCount++
			}
		}
	}

	var ignoreGlobs []string
	if rm != nil {
		ignoreGlobs = rm.Ignore
	}
	return &PageData{
		IssueLink:       issue.Link(),
		ToggleURL:       issue.Link() + "/review-map/progress",
		Bullets:         res.Bullets,
		CommentLinks:    comments,
		Coverage:        res.Coverage,
		IgnoreGlobs:     ignoreGlobs,
		InvalidRefCount: res.InvalidRefCount,
		OrphanCount:     res.OrphanCount,
		Progress:        progressList,
		BulletFlags:     bulletFlags,
		FlaggedCount:    flaggedCount,
	}
}

// PreparePageData assembles the page data for an issue, doing all I/O. It returns
// nil when the feature is disabled, the description has no review-map block, or
// anything fails on the way, so the page falls back to rendering upstream-style.
func PreparePageData(ctx context.Context, doer *user_model.User, issue *issues_model.Issue) *PageData {
	if !setting.ReviewMap.Enabled || !issue.IsPull {
		return nil
	}

	block, status := reviewmapmod.ExtractBlock(issue.Content, setting.ReviewMap.MaxBlockBytes)
	switch status {
	case reviewmapmod.BlockNone:
		return nil
	case reviewmapmod.BlockTooLarge:
		return &PageData{IgnoredReason: IgnoredReasonTooLarge}
	}
	m, err := reviewmapmod.Parse(block, reviewmapmod.Limits{
		MaxBullets:        setting.ReviewMap.MaxBullets,
		MaxFilesPerBullet: setting.ReviewMap.MaxFilesPerBullet,
		MaxBlockBytes:     setting.ReviewMap.MaxBlockBytes,
	})
	if err != nil {
		return &PageData{IgnoredReason: IgnoredReasonMalformed}
	}

	if err := issue.LoadRepo(ctx); err != nil {
		log.Warn("PreparePageData: LoadRepo: %v", err)
		return nil
	}
	if err := issue.LoadPullRequest(ctx); err != nil {
		log.Warn("PreparePageData: LoadPullRequest: %v", err)
		return nil
	}
	pr := issue.PullRequest
	if err := pr.LoadBaseRepo(ctx); err != nil {
		log.Warn("PreparePageData: LoadBaseRepo: %v", err)
		return nil
	}
	gitRepo, closer, err := gitrepo.RepositoryFromContextOrOpen(ctx, pr.BaseRepo)
	if err != nil {
		log.Warn("PreparePageData: RepositoryFromContextOrOpen: %v", err)
		return nil
	}
	defer closer.Close()

	headCommitID, err := gitRepo.GetRefCommitID(pr.GetGitHeadRefName())
	if err != nil {
		log.Warn("PreparePageData: GetRefCommitID: %v", err)
		return nil
	}
	diff, err := gitdiff.GetDiffForAPI(ctx, gitRepo, &gitdiff.DiffOptions{
		BeforeCommitID: pr.MergeBase,
		AfterCommitID:  headCommitID,
		MaxLines:       -1,
		// MaxLineCharacters must stay positive: ParsePatch slices lines at this length
		MaxLineCharacters: setting.Git.MaxGitDiffLineCharacters,
		MaxFiles:          -1,
	})
	if err != nil {
		log.Warn("PreparePageData: GetDiffForAPI: %v", err)
		return nil
	}

	res := ComputeResult(m, diff, reviewmapmod.ExtractDescriptionBullets(issue.Content))

	issueLink := issue.Link()
	commentLinks := make(map[string][]CommentLink)
	for _, b := range res.Bullets {
		ref := firstValidRef(b.Refs)
		if ref == nil {
			continue
		}
		comments, err := issues_model.FetchCodeCommentsByLine(ctx, issue, doer, ref.File, int64(ref.StartLine), false)
		if err != nil {
			continue // skip silently: the bullet just gets fewer links
		}
		for _, c := range comments {
			commentLinks[b.Hash] = append(commentLinks[b.Hash], CommentLink{
				ID:  c.ID,
				URL: fmt.Sprintf("%s/files#issuecomment-%d", issueLink, c.ID),
			})
		}
	}

	progress, err := issues_model.GetProgress(ctx, issue.ID)
	if err != nil {
		log.Warn("PreparePageData: GetProgress: %v", err)
		return nil
	}
	userIDs := make([]int64, 0, len(progress))
	for userID := range progress {
		userIDs = append(userIDs, userID)
	}
	foundUsers, err := user_model.GetUserByIDs(ctx, userIDs)
	if err != nil {
		log.Warn("PreparePageData: GetUserByIDs: %v", err)
		return nil
	}
	users := make(map[int64]*user_model.User, len(foundUsers))
	for _, u := range foundUsers {
		users[u.ID] = u
	}

	pageData := BuildPageData(issue, res, m, progress, users, commentLinks)
	pageData.CanReview = doer != nil
	if doer != nil {
		pageData.MyReviewed = myReviewedHashes(progress, doer.ID)
		pageData.MyApprovedRefs = myApprovedRefKeys(progress, doer.ID)
		pageData.BulletApproval = BuildBulletApproval(res.Bullets, progress[doer.ID])
		pageData.MyFlagged = myFlaggedHashes(res.Bullets, progress[doer.ID])
	} else {
		// templates index this slice for every bullet: it must never be nil (out-of-range errors)
		pageData.BulletApproval = make([]ApprovalCount, len(res.Bullets))
		pageData.BulletFlags = make([]int, len(res.Bullets))
	}
	return pageData
}

// myFlaggedHashes collects the bullet hashes the given user flagged for discussion.
func myFlaggedHashes(bullets []BulletStatus, keys map[string]bool) []string {
	var flagged []string
	for _, b := range bullets {
		if keys[FlagProgressKey(b.Hash)] {
			flagged = append(flagged, b.Hash)
		}
	}
	return flagged
}

// myReviewedHashes collects the reviewed bullet hashes of one user, sorted for stable output. Pure: no I/O.
func myReviewedHashes(progress map[int64]map[string]bool, doerID int64) []string {
	hashes := make([]string, 0, len(progress[doerID]))
	for hash, reviewed := range progress[doerID] {
		if reviewed && !strings.HasSuffix(hash, ":flag") {
			hashes = append(hashes, hash)
		}
	}
	slices.Sort(hashes)
	return hashes
}

// myApprovedRefKeys collects the approved per-ref progress keys of one user, sorted for stable output. Pure: no I/O.
func myApprovedRefKeys(progress map[int64]map[string]bool, doerID int64) []string {
	keys := make([]string, 0, len(progress[doerID]))
	for key, approved := range progress[doerID] {
		if approved && isRefProgressKey(key) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func isRefProgressKey(key string) bool {
	hash, rest, ok := strings.Cut(key, ":")
	return ok && len(hash) == 40 && strings.HasPrefix(rest, "i")
}

// bulletCompleted reports whether every valid ref of the bullet is approved in keys, or the
// bullet hash itself is. Bullets without valid refs are only completable via their hash.
func bulletCompleted(b BulletStatus, keys map[string]bool) bool {
	if keys[b.Hash] {
		return true
	}
	total, approved := 0, 0
	for i, ref := range b.Refs {
		if ref.Valid {
			total++
			if keys[RefProgressKey(b.Hash, i)] {
				approved++
			}
		}
	}
	return total > 0 && approved == total
}

// BuildBulletApproval computes the signed-in user's per-bullet "k/N approved" over valid refs. Pure: no I/O.
func BuildBulletApproval(bullets []BulletStatus, keys map[string]bool) []ApprovalCount {
	if keys == nil {
		return nil
	}
	approval := make([]ApprovalCount, len(bullets))
	for bi, b := range bullets {
		for i, ref := range b.Refs {
			if ref.Valid {
				approval[bi].Total++
				if keys[RefProgressKey(b.Hash, i)] {
					approval[bi].Approved++
				}
			}
		}
	}
	return approval
}

func firstValidRef(refs []RefStatus) *RefStatus {
	for i := range refs {
		if refs[i].Valid {
			return &refs[i]
		}
	}
	return nil
}
