// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issues_test

import (
	"strings"
	"testing"

	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewMapProgress(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	t.Run("reject too long hash", func(t *testing.T) {
		require.Error(t, issues_model.SetBulletReviewed(t.Context(), 2, 4, strings.Repeat("a", 65), true))
		unittest.AssertNotExistsBean(t, &issues_model.ReviewMapProgress{UserID: 2, IssueID: 4})
	})

	t.Run("insert, toggle off and user isolation", func(t *testing.T) {
		require.NoError(t, issues_model.SetBulletReviewed(t.Context(), 2, 4, "hash1", true))
		require.NoError(t, issues_model.SetBulletReviewed(t.Context(), 2, 4, "hash2", true))

		progress, err := issues_model.GetProgress(t.Context(), 4)
		require.NoError(t, err)
		assert.Equal(t, map[int64]map[string]bool{2: {"hash1": true, "hash2": true}}, progress)
		unittest.AssertCount(t, &issues_model.ReviewMapProgress{UserID: 2, IssueID: 4}, 1)

		// toggling off removes only that hash, other users stay isolated
		require.NoError(t, issues_model.SetBulletReviewed(t.Context(), 2, 4, "hash1", false))
		require.NoError(t, issues_model.SetBulletReviewed(t.Context(), 3, 4, "hash3", true))

		progress, err = issues_model.GetProgress(t.Context(), 4)
		require.NoError(t, err)
		assert.Equal(t, map[int64]map[string]bool{2: {"hash2": true}, 3: {"hash3": true}}, progress)

		// a user with zero hashes left is skipped
		require.NoError(t, issues_model.SetBulletReviewed(t.Context(), 2, 4, "hash2", false))
		progress, err = issues_model.GetProgress(t.Context(), 4)
		require.NoError(t, err)
		assert.Equal(t, map[int64]map[string]bool{3: {"hash3": true}}, progress)
	})

	t.Run("upsert existing row", func(t *testing.T) {
		before := unittest.AssertExistsAndLoadBean(t, &issues_model.ReviewMapProgress{UserID: 2, IssueID: 1})

		require.NoError(t, issues_model.SetBulletReviewed(t.Context(), 2, 1, "hash4", true))

		after := unittest.AssertExistsAndLoadBean(t, &issues_model.ReviewMapProgress{UserID: 2, IssueID: 1})
		assert.EqualValues(t, before.ID, after.ID)
		assert.Equal(t, map[string]bool{"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0": true, "hash4": true}, after.Reviewed)
		assert.Greater(t, after.UpdatedUnix, before.UpdatedUnix)
		unittest.AssertCount(t, &issues_model.ReviewMapProgress{UserID: 2, IssueID: 1}, 1)

		progress, err := issues_model.GetProgress(t.Context(), 1)
		require.NoError(t, err)
		assert.Equal(t, map[int64]map[string]bool{2: {"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0": true, "hash4": true}}, progress)
	})

	t.Run("no progress returns empty map", func(t *testing.T) {
		progress, err := issues_model.GetProgress(t.Context(), 999)
		require.NoError(t, err)
		assert.NotNil(t, progress)
		assert.Empty(t, progress)
	})
}
