// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	"gitea.dev/modules/timeutil"

	"xorm.io/builder"
)

// ReviewMapProgress stores which PR-description bullets a user has already reviewed
type ReviewMapProgress struct {
	ID          int64              `xorm:"pk autoincr"`
	UserID      int64              `xorm:"NOT NULL UNIQUE(s)"`
	IssueID     int64              `xorm:"NOT NULL INDEX UNIQUE(s)"`
	Reviewed    map[string]bool    `xorm:"NOT NULL LONGTEXT JSON"` // bulletHash -> reviewed
	UpdatedUnix timeutil.TimeStamp `xorm:"updated"`
}

func init() {
	db.RegisterModel(new(ReviewMapProgress))
}

// SetBulletReviewed upserts the reviewed state of one bullet hash (reviewed=false removes it).
func SetBulletReviewed(ctx context.Context, userID, issueID int64, bulletHash string, reviewed bool) error {
	if len(bulletHash) > 64 {
		return fmt.Errorf("SetBulletReviewed: bullet hash too long: %d", len(bulletHash))
	}
	return db.WithTx(ctx, func(ctx context.Context) error {
		progress, exists, err := db.Get[ReviewMapProgress](ctx, builder.Eq{"user_id": userID, "issue_id": issueID})
		if err != nil {
			return err
		}
		engine := db.GetEngine(ctx)
		if !exists {
			if !reviewed {
				return nil
			}
			progress = &ReviewMapProgress{UserID: userID, IssueID: issueID, Reviewed: map[string]bool{bulletHash: true}}
			_, err := engine.Insert(progress)
			return err
		}
		if progress.Reviewed == nil {
			progress.Reviewed = map[string]bool{}
		}
		if reviewed {
			progress.Reviewed[bulletHash] = true
		} else {
			delete(progress.Reviewed, bulletHash)
		}
		_, err = engine.ID(progress.ID).Cols("reviewed").Update(progress)
		return err
	})
}

// GetProgress returns the reviewed bullet hashes of an issue as a map from userID to the set of reviewed hashes.
func GetProgress(ctx context.Context, issueID int64) (map[int64]map[string]bool, error) {
	var progresses []*ReviewMapProgress
	if err := db.GetEngine(ctx).Where(builder.Eq{"issue_id": issueID}).Find(&progresses); err != nil {
		return nil, err
	}
	progressMap := make(map[int64]map[string]bool)
	for _, progress := range progresses {
		hashes := make(map[string]bool, len(progress.Reviewed))
		for hash, reviewed := range progress.Reviewed {
			if reviewed {
				hashes[hash] = true
			}
		}
		if len(hashes) > 0 {
			progressMap[progress.UserID] = hashes
		}
	}
	return progressMap, nil
}
