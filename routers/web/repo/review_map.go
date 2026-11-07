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
)

// reviewMapProgressUpdate Struct to parse the body of a request to update the reviewed bullets of a PR
type reviewMapProgressUpdate struct {
	Hash     string `json:"hash"`
	Reviewed bool   `json:"reviewed"`
}

// UpdateReviewMapProgress marks one bullet of a PR description as reviewed (or not) by the doer
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
	if len(data.Hash) == 0 || len(data.Hash) > 64 {
		ctx.Resp.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := issues_model.SetBulletReviewed(ctx, ctx.Doer.ID, issue.ID, data.Hash, data.Reviewed); err != nil {
		ctx.ServerError("SetBulletReviewed", err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
