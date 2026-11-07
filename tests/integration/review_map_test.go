// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/unittest"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

func TestReviewMapProgressEndpoint(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// user2/repo1 pull #3 is issue ID 3 in the fixtures, the seeded progress row
	// belongs to issue ID 1 and must not interfere
	urlStr := "/user2/repo1/pulls/3/review-map/progress"
	sum := sha1.Sum([]byte("x"))
	hash := hex.EncodeToString(sum[:])

	session := loginUser(t, "user2")

	req := NewRequestWithJSON(t, "POST", urlStr, map[string]any{"hash": hash, "reviewed": true})
	session.MakeRequest(t, req, http.StatusNoContent)

	progress := unittest.AssertExistsAndLoadBean(t, &issues_model.ReviewMapProgress{UserID: 2, IssueID: 3})
	assert.Equal(t, map[string]bool{hash: true}, progress.Reviewed)

	// toggling off empties the reviewed map but keeps the row
	req = NewRequestWithJSON(t, "POST", urlStr, map[string]any{"hash": hash, "reviewed": false})
	session.MakeRequest(t, req, http.StatusNoContent)

	progress = unittest.AssertExistsAndLoadBean(t, &issues_model.ReviewMapProgress{UserID: 2, IssueID: 3})
	assert.Empty(t, progress.Reviewed)

	req = NewRequestWithBody(t, "POST", urlStr, strings.NewReader("not json")).
		SetHeader("Content-Type", "application/json")
	session.MakeRequest(t, req, http.StatusBadRequest)

	req = NewRequestWithJSON(t, "POST", urlStr, map[string]any{"hash": "", "reviewed": true})
	session.MakeRequest(t, req, http.StatusBadRequest)

	req = NewRequestWithJSON(t, "POST", urlStr, map[string]any{"hash": strings.Repeat("a", 65), "reviewed": true})
	session.MakeRequest(t, req, http.StatusBadRequest)

	// anonymous requests are rejected by the sign-in middleware
	req = NewRequestWithJSON(t, "POST", urlStr, map[string]any{"hash": hash, "reviewed": true})
	emptyTestSession(t).MakeRequest(t, req, http.StatusSeeOther)
}
