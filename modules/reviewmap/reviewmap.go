// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package reviewmap

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gitea.dev/modules/glob"
)

type Limits struct {
	MaxBullets, MaxFilesPerBullet, MaxBlockBytes int
}

const (
	maxSummaryLen  = 280 // summary/note character cap
	maxDetailLines = 8   // details items per bullet
	maxDetailLen   = 200 // characters per details item
)

type FileRange struct {
	File               string
	StartLine, EndLine int // inclusive, new-side
	Note               string
}

type Bullet struct {
	Text    string
	Summary string
	Details []string
	Changes []FileRange
}

type Map struct {
	Ignore  []string
	Bullets []Bullet
}

const (
	BlockNone = iota
	BlockOK
	BlockTooLarge
)

const (
	maxFileLength  = 500
	maxLineNumber  = 100_000_000
	reviewMapFence = "```review-map"
)

// ExtractBlock finds the FIRST ```review-map fenced block in the markdown body.
// maxBlockBytes caps the bytes between the fences; exceeding → BlockTooLarge.
func ExtractBlock(content string, maxBlockBytes int) (string, int) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimRight(line, " \t\r") != reviewMapFence {
			continue
		}
		inner, closed := collectBlockLines(lines[i+1:])
		if !closed {
			return "", BlockNone
		}
		block := strings.Join(inner, "\n")
		if len(block) > maxBlockBytes {
			return "", BlockTooLarge
		}
		return block, BlockOK
	}
	return "", BlockNone
}

// collectBlockLines returns the lines until the next line starting with "```";
// closed reports whether such an end fence exists.
func collectBlockLines(lines []string) (inner []string, closed bool) {
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			return inner, true
		}
		inner = append(inner, line)
	}
	return nil, false
}

// ExtractDescriptionBullets returns top-level markdown list-item texts from the
// body, skipping fenced code blocks (so the review-map block itself is excluded).
func ExtractDescriptionBullets(content string) []string {
	var bullets []string
	inFence := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		var text string
		switch {
		case strings.HasPrefix(line, "- "), strings.HasPrefix(line, "* "), strings.HasPrefix(line, "+ "):
			text = line[2:]
		}
		if text = strings.TrimSpace(text); text != "" {
			bullets = append(bullets, text)
		}
	}
	return bullets
}

// Parse reads the restricted subset. Any deviation (unknown key, wrong indent,
// anchor/alias/tag-like value, bad range, >limits, invalid glob) → error →
// caller ignores the whole block.
func Parse(block string, limits Limits) (*Map, error) {
	p := &blockParser{m: &Map{Ignore: []string{}, Bullets: []Bullet{}}, limits: limits, seenKeys: map[string]bool{}, seenTexts: map[string]bool{}}
	for i, raw := range strings.Split(block, "\n") {
		if err := p.parseLine(strings.TrimRight(raw, " \t\r")); err != nil {
			return nil, fmt.Errorf("Parse: line %d: %w", i+1, err)
		}
	}
	if err := p.finish(); err != nil {
		return nil, fmt.Errorf("Parse: %w", err)
	}
	return p.m, nil
}

type blockParser struct {
	m         *Map
	limits    Limits
	seenKeys  map[string]bool
	seenTexts map[string]bool

	section       string // "", "ignore" or "bullets"
	curBullet     *Bullet
	inChanges     bool
	curRange      *FileRange
	curRangeReady bool
	inDetails     bool // inside a bullet's "details:" list
	hasSummary    bool
	hasDetails    bool
}

func (p *blockParser) parseLine(line string) error {
	if line == "" {
		return nil
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	switch indent {
	case 0:
		return p.parseTopLevelLine(line)
	case 2:
		return p.parseListItemLine(line[2:])
	case 4:
		return p.parseChangesLine(line[4:])
	case 6:
		return p.parseFileLine(line[6:])
	case 8:
		return p.parseLinesLine(line[8:])
	default:
		return fmt.Errorf("wrong indentation (%d spaces)", indent)
	}
}

func (p *blockParser) parseTopLevelLine(line string) error {
	switch {
	case line == "ignore:" || line == "bullets:":
		if p.hasPendingRange() {
			return errors.New(`missing "lines" for the previous file entry`)
		}
		key := line[:len(line)-1]
		if p.seenKeys[key] {
			return fmt.Errorf("duplicate top-level key %q", key)
		}
		p.seenKeys[key] = true
		p.section, p.curBullet, p.inChanges, p.curRange = key, nil, false, nil
		p.inDetails, p.hasSummary, p.hasDetails = false, false, false
		return nil
	case strings.HasPrefix(line, "ignore:") || strings.HasPrefix(line, "bullets:"):
		return fmt.Errorf("top-level key must not carry a value: %q", line)
	default:
		return fmt.Errorf("unknown top-level content %q", line)
	}
}

func (p *blockParser) parseListItemLine(item string) error {
	if !strings.HasPrefix(item, "- ") {
		return fmt.Errorf(`expected a list item "- ", got %q`, item)
	}
	rest := item[2:]
	switch p.section {
	case "":
		return errors.New("list item outside of a section")
	case "ignore":
		pattern, err := parseQuotedString(rest)
		if err != nil {
			return err
		}
		if _, err := glob.Compile(pattern, '/'); err != nil {
			return fmt.Errorf("invalid ignore glob %q: %w", pattern, err)
		}
		p.m.Ignore = append(p.m.Ignore, pattern)
		return nil
	default: // "bullets"
		return p.parseBulletItem(rest)
	}
}

func (p *blockParser) parseBulletItem(rest string) error {
	if p.hasPendingRange() {
		return errors.New(`missing "lines" for the previous file entry`)
	}
	if !strings.HasPrefix(rest, "bullet: ") {
		return fmt.Errorf(`expected "bullet: <quoted text>", got %q`, rest)
	}
	text, err := parseQuotedString(rest[len("bullet: "):])
	if err != nil {
		return err
	}
	if text == "" {
		return errors.New("bullet text must not be empty")
	}
	if p.seenTexts[text] {
		return fmt.Errorf("duplicate bullet text %q", text)
	}
	p.seenTexts[text] = true
	p.m.Bullets = append(p.m.Bullets, Bullet{Text: text, Changes: []FileRange{}})
	if len(p.m.Bullets) > p.limits.MaxBullets {
		return fmt.Errorf("too many bullets (%d > limit %d)", len(p.m.Bullets), p.limits.MaxBullets)
	}
	p.curBullet, p.inChanges, p.curRange = &p.m.Bullets[len(p.m.Bullets)-1], false, nil
	p.inDetails, p.hasSummary, p.hasDetails = false, false, false
	return nil
}

func (p *blockParser) parseChangesLine(line string) error {
	switch {
	case line == "changes:":
		if p.curBullet == nil {
			return errors.New(`"changes:" without a bullet`)
		}
		if p.hasPendingRange() {
			return errors.New(`missing "lines" for the previous file entry`)
		}
		if p.inChanges {
			return errors.New(`duplicate "changes:" key for a bullet`)
		}
		p.inChanges, p.inDetails = true, false
		p.curRange = nil
		return nil
	case strings.HasPrefix(line, "summary: "):
		if p.curBullet == nil {
			return errors.New(`"summary:" without a bullet`)
		}
		if p.hasPendingRange() {
			return errors.New(`missing "lines" for the previous file entry`)
		}
		if p.hasSummary {
			return errors.New(`duplicate "summary:" key for a bullet`)
		}
		summary, err := parseQuotedString(line[len("summary: "):])
		if err != nil {
			return err
		}
		if len(summary) > maxSummaryLen {
			return fmt.Errorf("summary must not exceed %d characters (got %d)", maxSummaryLen, len(summary))
		}
		p.curBullet.Summary, p.hasSummary, p.inDetails = summary, true, false
		return nil
	case line == "details:":
		if p.curBullet == nil {
			return errors.New(`"details:" without a bullet`)
		}
		if p.hasPendingRange() {
			return errors.New(`missing "lines" for the previous file entry`)
		}
		if p.hasDetails {
			return errors.New(`duplicate "details:" key for a bullet`)
		}
		if p.curBullet.Details == nil {
			p.curBullet.Details = []string{}
		}
		p.hasDetails, p.inDetails = true, true
		return nil
	default:
		return fmt.Errorf(`expected "changes:", "summary:" or "details:", got %q`, line)
	}
}

func (p *blockParser) parseFileLine(line string) error {
	if p.inDetails {
		if !strings.HasPrefix(line, "- ") {
			return fmt.Errorf(`expected a details item "- ", got %q`, line)
		}
		item, err := parseQuotedString(line[2:])
		if err != nil {
			return err
		}
		if len(item) > maxDetailLen {
			return fmt.Errorf("details item must not exceed %d characters (got %d)", maxDetailLen, len(item))
		}
		p.curBullet.Details = append(p.curBullet.Details, item)
		if len(p.curBullet.Details) > maxDetailLines {
			return fmt.Errorf("too many details items (%d > limit %d)", len(p.curBullet.Details), maxDetailLines)
		}
		return nil
	}
	if !strings.HasPrefix(line, "- file:") {
		return fmt.Errorf(`expected "- file: <path>", got %q`, line)
	}
	if !p.inChanges {
		return errors.New(`file entry without "changes:"`)
	}
	if p.hasPendingRange() {
		return errors.New(`missing "lines" for the previous file entry`)
	}
	value := line[len("- file:"):]
	if value == "" || value[0] != ' ' {
		return fmt.Errorf(`expected "- file: <path>", got %q`, line)
	}
	if strings.HasPrefix(value[1:], " ") {
		return fmt.Errorf(`file path must not start with a space, got %q`, line)
	}
	file, err := parseFilePath(value[1:])
	if err != nil {
		return err
	}
	p.curBullet.Changes = append(p.curBullet.Changes, FileRange{File: file})
	if len(p.curBullet.Changes) > p.limits.MaxFilesPerBullet {
		return fmt.Errorf("too many file entries for a bullet (%d > limit %d)", len(p.curBullet.Changes), p.limits.MaxFilesPerBullet)
	}
	p.curRange, p.curRangeReady = &p.curBullet.Changes[len(p.curBullet.Changes)-1], false
	return nil
}

func (p *blockParser) parseLinesLine(line string) error {
	if strings.HasPrefix(line, "note: ") {
		if p.curRange == nil || !p.curRangeReady {
			return errors.New(`"note:" must follow the "lines:" of a file entry`)
		}
		note, err := parseQuotedString(line[len("note: "):])
		if err != nil {
			return err
		}
		if len(note) > maxSummaryLen {
			return fmt.Errorf("note must not exceed %d characters (got %d)", maxSummaryLen, len(note))
		}
		p.curRange.Note, p.inDetails = note, false
		return nil
	}
	if !strings.HasPrefix(line, "lines:") {
		return fmt.Errorf(`expected "lines: <N> or <N>-<M>", got %q`, line)
	}
	if p.curRange == nil {
		return errors.New(`"lines:" without a file entry`)
	}
	if p.curRangeReady {
		return errors.New(`duplicate "lines:" key for a file entry`)
	}
	value := line[len("lines:"):]
	if value == "" || value[0] != ' ' {
		return fmt.Errorf(`expected "lines: <N> or <N>-<M>", got %q`, line)
	}
	start, end, err := parseLineRange(value[1:])
	if err != nil {
		return err
	}
	p.curRange.StartLine, p.curRange.EndLine = start, end
	p.curRangeReady = true
	return nil
}

func (p *blockParser) finish() error {
	if p.hasPendingRange() {
		return errors.New(`missing "lines" for the previous file entry`)
	}
	return nil
}

func (p *blockParser) hasPendingRange() bool {
	return p.curRange != nil && !p.curRangeReady
}

func parseQuotedString(s string) (string, error) {
	if s == "" || s[0] != '"' {
		if s != "" {
			if err := rejectYAMLMarker(s); err != nil {
				return "", err
			}
		}
		return "", fmt.Errorf("value must be a double-quoted string, got %q", s)
	}
	if len(s) < 2 || s[len(s)-1] != '"' || strings.Contains(s[1:len(s)-1], `"`) {
		return "", fmt.Errorf("invalid double-quoted string %q", s)
	}
	return s[1 : len(s)-1], nil
}

func parseFilePath(v string) (string, error) {
	if v == "" {
		return "", errors.New("empty file path")
	}
	if v[0] == '"' {
		quoted, err := parseQuotedString(v)
		if err != nil {
			return "", err
		}
		v = quoted
	} else if err := rejectYAMLMarker(v); err != nil {
		return "", err
	}
	switch {
	case len(v) > maxFileLength:
		return "", fmt.Errorf("file path too long (%d > %d)", len(v), maxFileLength)
	case v == "":
		return "", errors.New("empty file path")
	case strings.HasPrefix(v, "/"):
		return "", fmt.Errorf("file path must be relative, got %q", v)
	case strings.Contains(v, "\\"):
		return "", fmt.Errorf("file path must not contain backslashes, got %q", v)
	}
	for _, seg := range strings.Split(v, "/") {
		if seg == ".." {
			return "", fmt.Errorf("file path must not contain \"..\" segments, got %q", v)
		}
	}
	return v, nil
}

// rejectYAMLMarker rejects raw values that YAML would read as anchors, aliases or tags.
func rejectYAMLMarker(v string) error {
	switch v[0] {
	case '&':
		return fmt.Errorf("anchor-like value %q", v)
	case '*':
		return fmt.Errorf("alias-like value %q", v)
	case '!':
		return fmt.Errorf("tag-like value %q", v)
	}
	return nil
}

func parseLineRange(s string) (start, end int, err error) {
	startStr, endStr, hasEnd := strings.Cut(s, "-")
	if start, err = parseLineNumber(startStr); err != nil {
		return 0, 0, err
	}
	end = start
	if hasEnd {
		if end, err = parseLineNumber(endStr); err != nil {
			return 0, 0, err
		}
	}
	if start < 1 || end < start {
		return 0, 0, fmt.Errorf("invalid line range %q", s)
	}
	if end > maxLineNumber {
		return 0, 0, fmt.Errorf("line range %q exceeds %d", s, maxLineNumber)
	}
	return start, end, nil
}

func parseLineNumber(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty line number")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid line number %q", s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid line number %q", s)
	}
	return n, nil
}
