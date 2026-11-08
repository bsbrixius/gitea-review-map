// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"

	"gitea.dev/services/context"
)

// ViewReviewMapTab renders the Review Map tab: the full change-card list with
// summaries, approvals and flags (the conversation page keeps a slim strip).
func ViewReviewMapTab(ctx *context.Context) {
	ctx.Data["PageIsPullList"] = true
	ctx.Data["PageIsPullReviewMap"] = true

	issue, ok := getPullInfo(ctx)
	if !ok {
		return
	}
	prViewInfo := newPullRequestViewInfo()
	prViewInfo.prepareViewInfo(ctx, issue) // fills NumCommits/NumFiles for the tab menu
	if ctx.Written() {
		return
	}
	PrepareReviewMapData(ctx, issue)
	if ctx.Written() {
		return
	}

	ctx.Data["HasIssuesOrPullsWritePermission"] = ctx.Repo.Permission.CanWriteIssuesOrPulls(issue.IsPull)
	ctx.Data["IsIssuePoster"] = ctx.IsSigned && issue.IsPoster(ctx.Doer.ID)

	ctx.HTML(http.StatusOK, "repo/pulls/reviewmap")
}
