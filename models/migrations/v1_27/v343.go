// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_27

import (
	"gitea.dev/models/db"
	"gitea.dev/modules/timeutil"
)

func AddReviewMapProgress(x db.EngineMigration) error {
	type ReviewMapProgress struct {
		ID          int64              `xorm:"pk autoincr"`
		UserID      int64              `xorm:"NOT NULL UNIQUE(s)"`
		IssueID     int64              `xorm:"NOT NULL INDEX UNIQUE(s)"`
		Reviewed    map[string]bool    `xorm:"NOT NULL LONGTEXT JSON"`
		UpdatedUnix timeutil.TimeStamp `xorm:"updated"`
	}
	return x.Sync(new(ReviewMapProgress))
}
