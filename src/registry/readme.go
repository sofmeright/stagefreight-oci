package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sofmeright/stagefreight/src/config"
)

// ReadmeContent holds the processed README ready for pushing to registries.
type ReadmeContent struct {
	Short string // max 100 chars
	Full  string // full markdown
}

// PrepareReadme loads, processes, and returns README content ready for registry sync.
func PrepareReadme(cfg config.DockerReadmeConfig, rootDir string) (*ReadmeContent, error) {
	// 1. Load file
	file := cfg.File
	if file == "" {
		file = "README.md"
	}

	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootDir, path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("readme: reading %s: %w", file, err)
	}
	content := string(raw)

	// 2. Extract markers
	if cfg.Markers != nil && *cfg.Markers {
		start := cfg.StartMarker
		if start == "" {
			start = "<!-- dockerhub-start -->"
		}
		end := cfg.EndMarker
		if end == "" {
			end = "<!-- dockerhub-end -->"
		}

		extracted, err := extractMarkers(content, start, end)
		if err != nil {
			return nil, err
		}
		content = extracted
	}

	// 3. Rewrite relative links
	if cfg.LinkBase != "" {
		content = rewriteRelativeLinks(content, cfg.LinkBase)
	}

	// 4. Apply regex transforms
	for _, t := range cfg.Transforms {
		re, err := regexp.Compile(t.Pattern)
		if err != nil {
			return nil, fmt.Errorf("readme: invalid transform pattern %q: %w", t.Pattern, err)
		}
		content = re.ReplaceAllString(content, t.Replace)
	}

	// 5. Generate short description
	short := cfg.Description
	if short == "" {
		short = extractFirstParagraph(content)
	}
	short = truncateAtWord(short, 100)

	return &ReadmeContent{
		Short: short,
		Full:  content,
	}, nil
}

// extractMarkers pulls content between start and end markers (exclusive of the markers themselves).
func extractMarkers(content, start, end string) (string, error) {
	startIdx := strings.Index(content, start)
	if startIdx < 0 {
		return "", fmt.Errorf("readme: start marker %q not found", start)
	}
	startIdx += len(start)

	endIdx := strings.Index(content[startIdx:], end)
	if endIdx < 0 {
		return "", fmt.Errorf("readme: end marker %q not found", end)
	}

	return strings.TrimSpace(content[startIdx : startIdx+endIdx]), nil
}

// rewriteRelativeLinks rewrites relative markdown links and images to absolute URLs.
// Skips http://, https://, /absolute, #anchor, and mailto: links.
var linkPattern = regexp.MustCompile(`(\[(?:[^\]]*)\]\()([^)]+)(\))`)

func rewriteRelativeLinks(content, base string) string {
	base = strings.TrimRight(base, "/")

	return linkPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := linkPattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}

		href := parts[2]

		// Skip absolute URLs, anchors, and mailto
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") ||
			strings.HasPrefix(href, "/") || strings.HasPrefix(href, "#") ||
			strings.HasPrefix(href, "mailto:") {
			return match
		}

		// Strip leading ./ if present
		href = strings.TrimPrefix(href, "./")

		return parts[1] + base + "/" + href + parts[3]
	})
}

// extractFirstParagraph returns the first prose paragraph from markdown content.
// Skips headings, badges (lines starting with [! or [![), HTML comments, and blank lines.
func extractFirstParagraph(content string) string {
	lines := strings.Split(content, "\n")
	var para []string
	inComment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track HTML comments
		if strings.Contains(trimmed, "<!--") {
			inComment = true
		}
		if strings.Contains(trimmed, "-->") {
			inComment = false
			continue
		}
		if inComment {
			continue
		}

		// Skip blank lines (but end paragraph collection if we have content)
		if trimmed == "" {
			if len(para) > 0 {
				break
			}
			continue
		}

		// Skip headings
		if strings.HasPrefix(trimmed, "#") {
			if len(para) > 0 {
				break
			}
			continue
		}

		// Skip badge lines
		if strings.HasPrefix(trimmed, "[![") || strings.HasPrefix(trimmed, "[!") {
			continue
		}

		// Skip HTML tags
		if strings.HasPrefix(trimmed, "<") && !strings.HasPrefix(trimmed, "<a") {
			continue
		}

		para = append(para, trimmed)
	}

	return strings.Join(para, " ")
}
