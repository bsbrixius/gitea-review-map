// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"

	issues_model "gitea.dev/models/issues"
	"gitea.dev/modules/json"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	"gitea.dev/services/context"
	"gitea.dev/services/reviewmap"
)

// reviewMapProgressUpdate Struct to parse the body of a request to update the reviewed bullets of a PR
type reviewMapProgressUpdate struct {
	Hash     string `json:"hash"`
	Reviewed bool   `json:"reviewed"`
	RefIndex *int   `json:"refIndex"` // >= 0: approve that one file reference instead of the whole bullet
}

// UpdateReviewMapProgress marks one bullet (or one of its file references) of a PR description
// as reviewed (or not) by the doer
func UpdateReviewMapProgress(ctx *context.Context) {
	if !setting.ReviewMap.Enabled {
		ctx.NotFound(nil)
		return
	}
	issue, ok := getPullInfo(ctx)
	if !ok {
		return
	}

	var data reviewMapProgressUpdate
	if err := json.NewDecoder(ctx.Req.Body).Decode(&data); err != nil {
		log.Warn("UpdateReviewMapProgress: could not parse request body: %v", err)
		ctx.Resp.WriteHeader(http.StatusBadRequest)
		return
	}
	key := data.Hash
	if data.RefIndex != nil {
		if *data.RefIndex < 0 {
			ctx.Resp.WriteHeader(http.StatusBadRequest)
			return
		}
		key = reviewmap.RefProgressKey(data.Hash, *data.RefIndex)
	}
	if len(key) == 0 || len(key) > 64 {
		ctx.Resp.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := issues_model.SetBulletReviewed(ctx, ctx.Doer.ID, issue.ID, key, data.Reviewed); err != nil {
		ctx.ServerError("SetBulletReviewed", err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// PrepareReviewMapData sets the review-map panel data shared by the PR conversation and files pages.
// Fail-safe: when the feature is off or anything fails, nothing is set and the pages render upstream-style.
func PrepareReviewMapData(ctx *context.Context, issue *issues_model.Issue) {
	if !setting.ReviewMap.Enabled {
		return
	}
	pd := reviewmap.PreparePageData(ctx, ctx.Doer, issue)
	if pd == nil {
		return
	}
	myReviewed := make(map[string]bool, len(pd.MyReviewed))
	for _, hash := range pd.MyReviewed {
		myReviewed[hash] = true
	}
	ctx.Data["ReviewMapData"] = pd
	ctx.Data["ReviewMapMyReviewed"] = myReviewed
	ctx.Data["ReviewMapUnmappedChanged"] = pd.Coverage.TotalChanged - pd.Coverage.MappedChanged // templates cannot do arithmetic
	ctx.PageData["reviewMap"] = pd
}
