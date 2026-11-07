// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package reviewmap

import (
	"cmp"
	"context"
	"fmt"
	"slices"

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
		progressList = append(progressList, UserProgress{UserID: userID, Name: u.DisplayName(), Done: done, Total: len(res.Bullets)})
	}
	slices.SortFunc(progressList, func(a, b UserProgress) int {
		return cmp.Or(cmp.Compare(b.Done, a.Done), cmp.Compare(a.Name, b.Name))
	})

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
	return pageData
}

func firstValidRef(refs []RefStatus) *RefStatus {
	for i := range refs {
		if refs[i].Valid {
			return &refs[i]
		}
	}
	return nil
}
