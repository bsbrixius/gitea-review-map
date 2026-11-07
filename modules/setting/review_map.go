// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

var ReviewMap = struct {
	Enabled           bool
	MaxBullets        int `ini:"MAX_BULLETS"`
	MaxFilesPerBullet int `ini:"MAX_FILES_PER_BULLET"`
	MaxBlockBytes     int `ini:"MAX_BLOCK_BYTES"`
}{
	Enabled:           true,
	MaxBullets:        50,
	MaxFilesPerBullet: 10,
	MaxBlockBytes:     4 * 1024,
}

func loadReviewMapFrom(rootCfg ConfigProvider) {
	mustMapSetting(rootCfg, "review_map", &ReviewMap)
}
