// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package reviewmap

import (
	"crypto/sha1"
	"encoding/hex"
	"slices"

	"gitea.dev/modules/git"
	"gitea.dev/modules/glob"
	reviewmapmod "gitea.dev/modules/reviewmap"
	"gitea.dev/services/gitdiff"
)

type RefStatus struct {
	File               string
	NameHash           string
	StartLine, EndLine int
	Valid              bool
}

type BulletStatus struct {
	Index      int
	Hash, Text string
	Orphan     bool
	Refs       []RefStatus
	// changed lines attributed to this bullet (earliest covering bullet wins an overlap)
	AddedLines, RemovedLines int
}

type Coverage struct {
	TotalChanged, MappedChanged int
	UnmappedFiles               []string
}

type Result struct {
	Bullets                      []BulletStatus
	InvalidRefCount, OrphanCount int
	Coverage                     Coverage
	IgnoreGlobs                  []string
}

// HashBullet is the stable per-user progress key: sha1 hex of the bullet text.
func HashBullet(text string) string {
	h := sha1.Sum([]byte(text))
	return hex.EncodeToString(h[:])
}

type changeCounts struct{ added, removed int }

type fileCoverage struct {
	path      string // new-side path: Name, falling back to the old name for deletions
	file      *gitdiff.DiffFile
	ignored   bool
	changed   map[int]changeCounts // changed lines anchored at each new-side position; 0 = deletion without a new-side anchor, never coverable
	hunkSpans [][2]int             // new-side [start, end] of each hunk
}

// ComputeResult validates a review map against the diff and computes coverage. Pure: no I/O.
func ComputeResult(m *reviewmapmod.Map, diff *gitdiff.Diff, descriptionBullets []string) *Result {
	var ignoreGlobs []string
	var bullets []reviewmapmod.Bullet
	if m != nil {
		ignoreGlobs, bullets = m.Ignore, m.Bullets
	}
	res := &Result{IgnoreGlobs: slices.Clone(ignoreGlobs)}

	ignoreMatchers := make([]glob.Glob, 0, len(ignoreGlobs))
	for _, pattern := range ignoreGlobs {
		if g, err := glob.CompileGitignore(pattern); err == nil { // Parse already rejects invalid globs
			ignoreMatchers = append(ignoreMatchers, g)
		}
	}

	rangesByPath := map[string][]reviewmapmod.FileRange{}
	for _, b := range bullets {
		for _, r := range b.Changes {
			rangesByPath[r.File] = append(rangesByPath[r.File], r)
		}
	}

	var files []*fileCoverage
	filesByPath := map[string]*fileCoverage{}
	if diff != nil {
		for _, f := range diff.Files {
			fc := &fileCoverage{path: diffFilePath(f), file: f, changed: map[int]changeCounts{}}
			for _, g := range ignoreMatchers {
				if g.Match(fc.path) {
					fc.ignored = true
					break
				}
			}
			lastRight := 0
			for _, sec := range f.Sections {
				singleLineHunk := false
				for _, l := range sec.Lines {
					switch l.Type {
					case gitdiff.DiffLineSection:
						if si := l.SectionInfo; si != nil {
							singleLineHunk = si.RightHunkSize == 0 // git omits ",count" when it is 1
							if !singleLineHunk {
								end := si.RightIdx + si.RightHunkSize - 1
								fc.hunkSpans = append(fc.hunkSpans, [2]int{si.RightIdx, end})
							}
						}
					case gitdiff.DiffLineAdd:
						fc.changed[l.RightIdx] = changeCounts{added: fc.changed[l.RightIdx].added + 1}
						lastRight = l.RightIdx
						if singleLineHunk { // the hunk is this single added line; pure-deletion sides ("+0,0") never get here
							fc.hunkSpans = append(fc.hunkSpans, [2]int{l.RightIdx, l.RightIdx})
							singleLineHunk = false
						}
					case gitdiff.DiffLineDel:
						c := fc.changed[lastRight] // deletions anchor to the nearest preceding new-side line
						c.removed++
						fc.changed[lastRight] = c
					case gitdiff.DiffLinePlain:
						lastRight = l.RightIdx
					}
				}
			}
			files = append(files, fc)
			filesByPath[fc.path] = fc
		}
	}

	for _, fc := range files {
		if fc.ignored || fc.file.IsIncomplete {
			continue
		}
		if fc.file.IsBin || fc.file.IsSubmodule { // no line totals, but an unreferenced file is still reported as unmapped
			if len(rangesByPath[fc.path]) == 0 {
				res.Coverage.UnmappedFiles = append(res.Coverage.UnmappedFiles, fc.path)
			}
			continue
		}
		res.Coverage.TotalChanged += fc.file.Addition + fc.file.Deletion
		unmapped := false
		// count every changed line whose anchor position falls inside a mapped
		// range, in the same units as TotalChanged so full coverage is reachable
		for pos, count := range fc.changed {
			if coveredByAny(rangesByPath[fc.path], pos) {
				res.Coverage.MappedChanged += count.added + count.removed
			} else {
				unmapped = true
			}
		}
		if unmapped {
			res.Coverage.UnmappedFiles = append(res.Coverage.UnmappedFiles, fc.path)
		}
	}

	descriptionSet := make(map[string]bool, len(descriptionBullets))
	for _, text := range descriptionBullets {
		descriptionSet[text] = true
	}

	res.Bullets = make([]BulletStatus, len(bullets))
	for i, b := range bullets {
		bs := &res.Bullets[i]
		bs.Index, bs.Hash, bs.Text = i, HashBullet(b.Text), b.Text
		bs.Orphan = !descriptionSet[b.Text]
		if bs.Orphan {
			res.OrphanCount++
		}
		bs.Refs = make([]RefStatus, len(b.Changes))
		for j, r := range b.Changes {
			ref := &bs.Refs[j]
			ref.File, ref.NameHash = r.File, git.HashFilePathForWebUI(r.File)
			ref.StartLine, ref.EndLine = r.StartLine, r.EndLine
			fc := filesByPath[r.File]
			ref.Valid = fc != nil && !fc.ignored && intersectsAnySpan(fc.hunkSpans, r.StartLine, r.EndLine)
			if !ref.Valid {
				res.InvalidRefCount++
			}
		}
	}
	for _, fc := range files {
		if fc.ignored || fc.file.IsIncomplete { // same universe as the coverage math
			continue
		}
		for pos, c := range fc.changed {
			if bi := firstCoveringBullet(bullets, fc.path, pos); bi >= 0 {
				res.Bullets[bi].AddedLines += c.added
				res.Bullets[bi].RemovedLines += c.removed
			}
		}
	}
	return res
}

func firstCoveringBullet(bullets []reviewmapmod.Bullet, path string, pos int) int {
	for i, b := range bullets {
		for _, r := range b.Changes {
			if r.File == path && r.StartLine <= pos && pos <= r.EndLine {
				return i
			}
		}
	}
	return -1
}

func diffFilePath(f *gitdiff.DiffFile) string {
	if f.Name == "" {
		return f.GetDiffFileName()
	}
	return f.Name
}

func coveredByAny(ranges []reviewmapmod.FileRange, pos int) bool {
	return slices.ContainsFunc(ranges, func(r reviewmapmod.FileRange) bool {
		return r.StartLine <= pos && pos <= r.EndLine
	})
}

func intersectsAnySpan(spans [][2]int, start, end int) bool {
	return slices.ContainsFunc(spans, func(s [2]int) bool {
		return start <= s[1] && end >= s[0] // empty spans (deleted files: end < start) never intersect
	})
}
