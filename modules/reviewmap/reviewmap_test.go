// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package reviewmap

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSpecExampleBlock = `ignore:
  - "package-lock.json"
  - "src/**/*.snap"
bullets:
  - bullet: "Added Advanced Query slice and selectors to manage state and criteria"
    changes:
      - file: src/store/slices/AdvancedQuery.ts
        lines: 1-120
      - file: src/store/selectors/AdvancedQuery.ts
        lines: 1-45
  - bullet: "Updated infinite row datasource to support fetching data from the Advanced Query endpoint"
    changes:
      - file: src/utilities/infiniteRowDatasource.ts
        lines: 30-88`

func testFence(inner string) string {
	return "```review-map\n" + inner + "\n```\n"
}

func TestExtractBlock(t *testing.T) {
	cases := []struct {
		name          string
		content       string
		maxBlockBytes int
		wantBlock     string
		wantStatus    int
	}{
		{
			name:       "no block",
			content:    "# Pull request\n\n- did things\n- more things\n",
			wantBlock:  "",
			wantStatus: BlockNone,
		},
		{
			name:          "spec example",
			content:       "Human description.\n\n" + testFence(testSpecExampleBlock) + "\nAfter the block.\n",
			maxBlockBytes: 4 * 1024,
			wantBlock:     testSpecExampleBlock,
			wantStatus:    BlockOK,
		},
		{
			name:          "too large",
			content:       testFence(testSpecExampleBlock),
			maxBlockBytes: 16,
			wantBlock:     "",
			wantStatus:    BlockTooLarge,
		},
		{
			name:          "exact size is ok",
			content:       testFence(testSpecExampleBlock),
			maxBlockBytes: len(testSpecExampleBlock),
			wantBlock:     testSpecExampleBlock,
			wantStatus:    BlockOK,
		},
		{
			name:          "two blocks, first wins",
			content:       testFence(testSpecExampleBlock) + testFence("ignore:\n  - \"other\"\n"),
			maxBlockBytes: 4 * 1024,
			wantBlock:     testSpecExampleBlock,
			wantStatus:    BlockOK,
		},
		{
			name:          "first block too large, second not used",
			content:       testFence(testSpecExampleBlock) + testFence("ignore:\n  - \"other\"\n"),
			maxBlockBytes: 16,
			wantBlock:     "",
			wantStatus:    BlockTooLarge,
		},
		{
			name:          "trailing spaces after fence",
			content:       "```review-map  \n" + testSpecExampleBlock + "\n```\n",
			maxBlockBytes: 4 * 1024,
			wantBlock:     testSpecExampleBlock,
			wantStatus:    BlockOK,
		},
		{
			name:          "other fenced blocks ignored",
			content:       "```go\nfunc x() {}\n```\n\n" + testFence(testSpecExampleBlock),
			maxBlockBytes: 4 * 1024,
			wantBlock:     testSpecExampleBlock,
			wantStatus:    BlockOK,
		},
		{
			name:          "only other fenced blocks",
			content:       "```go\nfunc x() {}\n```\n",
			maxBlockBytes: 4 * 1024,
			wantBlock:     "",
			wantStatus:    BlockNone,
		},
		{
			name:          "unclosed fence",
			content:       "```review-map\nignore:\n  - \"x\"\n",
			maxBlockBytes: 4 * 1024,
			wantBlock:     "",
			wantStatus:    BlockNone,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			block, status := ExtractBlock(c.content, c.maxBlockBytes)
			assert.Equal(t, c.wantBlock, block)
			assert.Equal(t, c.wantStatus, status)
		})
	}
}

func testLimits() Limits {
	return Limits{MaxBullets: 50, MaxFilesPerBullet: 10, MaxBlockBytes: 4 * 1024}
}

func TestParse(t *testing.T) {
	t.Run("spec example", func(t *testing.T) {
		m, err := Parse(testSpecExampleBlock, testLimits())
		require.NoError(t, err)
		assert.Equal(t, &Map{
			Ignore: []string{"package-lock.json", "src/**/*.snap"},
			Bullets: []Bullet{
				{
					Text: "Added Advanced Query slice and selectors to manage state and criteria",
					Changes: []FileRange{
						{File: "src/store/slices/AdvancedQuery.ts", StartLine: 1, EndLine: 120},
						{File: "src/store/selectors/AdvancedQuery.ts", StartLine: 1, EndLine: 45},
					},
				},
				{
					Text: "Updated infinite row datasource to support fetching data from the Advanced Query endpoint",
					Changes: []FileRange{
						{File: "src/utilities/infiniteRowDatasource.ts", StartLine: 30, EndLine: 88},
					},
				},
			},
		}, m)
	})

	t.Run("single line range", func(t *testing.T) {
		m, err := Parse("bullets:\n  - bullet: \"single\"\n    changes:\n      - file: a.go\n        lines: 42\n", testLimits())
		require.NoError(t, err)
		assert.Equal(t, FileRange{File: "a.go", StartLine: 42, EndLine: 42}, m.Bullets[0].Changes[0])
	})

	t.Run("zero bullets", func(t *testing.T) {
		m, err := Parse("bullets:\n", testLimits())
		require.NoError(t, err)
		assert.Equal(t, &Map{Ignore: []string{}, Bullets: []Bullet{}}, m)
	})

	t.Run("empty block", func(t *testing.T) {
		m, err := Parse("", testLimits())
		require.NoError(t, err)
		assert.Equal(t, &Map{Ignore: []string{}, Bullets: []Bullet{}}, m)
	})

	t.Run("bullet without changes", func(t *testing.T) {
		m, err := Parse("bullets:\n  - bullet: \"orphan\"\n", testLimits())
		require.NoError(t, err)
		assert.Equal(t, &Map{
			Ignore:  []string{},
			Bullets: []Bullet{{Text: "orphan", Changes: []FileRange{}}},
		}, m)
	})

	t.Run("sections in reverse order with blank lines", func(t *testing.T) {
		m, err := Parse("bullets:\n  - bullet: \"x\"\n\nignore:\n\n  - \"*.log\"\n", testLimits())
		require.NoError(t, err)
		assert.Equal(t, &Map{
			Ignore:  []string{"*.log"},
			Bullets: []Bullet{{Text: "x", Changes: []FileRange{}}},
		}, m)
	})

	t.Run("quoted file path", func(t *testing.T) {
		m, err := Parse("bullets:\n  - bullet: \"x\"\n    changes:\n      - file: \"src/a b.ts\"\n        lines: 1-2\n", testLimits())
		require.NoError(t, err)
		assert.Equal(t, "src/a b.ts", m.Bullets[0].Changes[0].File)
	})
}

func makeBulletsBlock(n int) string {
	var sb strings.Builder
	sb.WriteString("bullets:\n")
	for i := range n {
		fmt.Fprintf(&sb, "  - bullet: \"b%d\"\n    changes:\n      - file: f%d.go\n        lines: 1-2\n", i, i)
	}
	return sb.String()
}

func makeFilesBlock(n int) string {
	var sb strings.Builder
	sb.WriteString("bullets:\n  - bullet: \"b\"\n    changes:\n")
	for i := range n {
		fmt.Fprintf(&sb, "      - file: f%d.go\n        lines: 1-2\n", i)
	}
	return sb.String()
}

func TestParseLimits(t *testing.T) {
	t.Run("bullets at the limit", func(t *testing.T) {
		_, err := Parse(makeBulletsBlock(50), testLimits())
		require.NoError(t, err)
	})

	t.Run("bullet over the limit", func(t *testing.T) {
		_, err := Parse(makeBulletsBlock(51), testLimits())
		require.Error(t, err)
	})

	t.Run("files at the limit", func(t *testing.T) {
		_, err := Parse(makeFilesBlock(10), testLimits())
		require.NoError(t, err)
	})

	t.Run("file over the limit", func(t *testing.T) {
		_, err := Parse(makeFilesBlock(11), testLimits())
		require.Error(t, err)
	})
}

func TestParseMalformed(t *testing.T) {
	cases := []struct {
		name  string
		block string
	}{
		{"bad indent", "ignore:\n   - \"x\"\n"},
		{"bad indent on changes", "bullets:\n  - bullet: \"x\"\n     changes:\n      - file: a.go\n        lines: 1\n"},
		{"unknown top-level key", "foo: 1\n"},
		{"unknown nested key", "bullets:\n  - bullet: \"x\"\n    notes: hi\n"},
		{"top-level key with value", "ignore: [\n"},
		{"bullet without quotes", "bullets:\n  - bullet: text\n"},
		{"ignore item without quotes", "ignore:\n  - x.snap\n"},
		{"alias value", "ignore:\n  - *alias\n"},
		{"anchor value", "bullets:\n  - bullet: &anchor\n"},
		{"tag-like file value", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: !tag\n        lines: 1\n"},
		{"unterminated quote", "bullets:\n  - bullet: \"abc\n"},
		{"trailing junk after quote", "ignore:\n  - \"a\" x\n"},
		{"reversed range", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: a.go\n        lines: 5-2\n"},
		{"zero line", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: a.go\n        lines: 0\n"},
		{"line over max", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: a.go\n        lines: 100000001\n"},
		{"range with spaces", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: a.go\n        lines: 1 - 5\n"},
		{"missing lines", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: a.go\n"},
		{"duplicate lines", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: a.go\n        lines: 1\n        lines: 2\n"},
		{"duplicate changes", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: a.go\n        lines: 1\n    changes:\n      - file: b.go\n        lines: 1\n"},
		{"duplicate top-level key", "ignore:\n  - \"a\"\nignore:\n  - \"b\"\n"},
		{"parent path", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: ../x\n        lines: 1\n"},
		{"nested parent path", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: src/../../x\n        lines: 1\n"},
		{"absolute path", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: /abs/x\n        lines: 1\n"},
		{"backslash path", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: src\\x.go\n        lines: 1\n"},
		{"path too long", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: " + strings.Repeat("a", 501) + "\n        lines: 1\n"},
		{"empty path", "bullets:\n  - bullet: \"x\"\n    changes:\n      - file: \n        lines: 1\n"},
		{"invalid glob", "ignore:\n  - \"[\"\n"},
		{"item outside section", "  - \"x\"\n"},
		{"changes without bullet", "    changes:\n"},
		{"file without changes", "      - file: a.go\n        lines: 1\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := Parse(c.block, testLimits())
			require.Error(t, err)
			assert.Nil(t, m)
			assert.ErrorContains(t, err, "Parse:")
		})
	}
}

func TestExtractDescriptionBullets(t *testing.T) {
	t.Run("dash, star and plus markers", func(t *testing.T) {
		assert.Equal(t, []string{"a", "b", "c"}, ExtractDescriptionBullets("- a\n* b\n+ c\n"))
	})

	t.Run("nested items are not top-level bullets", func(t *testing.T) {
		content := "- parent\n  - child\n    - grandchild\n* another parent\n"
		assert.Equal(t, []string{"parent", "another parent"}, ExtractDescriptionBullets(content))
	})

	t.Run("fenced content skipped", func(t *testing.T) {
		content := "- before\n" + testFence(testSpecExampleBlock) + "- after\n```go\n- not a bullet\n```\n"
		assert.Equal(t, []string{"before", "after"}, ExtractDescriptionBullets(content))
	})

	t.Run("numbered items are not bullets", func(t *testing.T) {
		assert.Empty(t, ExtractDescriptionBullets("1. first\n2. second\n"))
	})

	t.Run("no bullets", func(t *testing.T) {
		assert.Empty(t, ExtractDescriptionBullets("Plain description without lists.\n"))
	})
}

// The skill file ships the example AI authors copy; this pins that its example
// still parses. "go test" runs with the package dir as cwd, so the repo-root
// relative path reaches across module directories.
func TestSkillExampleParses(t *testing.T) {
	skill, err := os.ReadFile("../../contrib/ai-skills/review-map/SKILL.md")
	require.NoError(t, err)

	block, status := ExtractBlock(string(skill), 64*1024)
	require.Equal(t, BlockOK, status)

	m, err := Parse(block, testLimits())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(m.Bullets), 2)
	assert.NotEmpty(t, m.Ignore)
}
