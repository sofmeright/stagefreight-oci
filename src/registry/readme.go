package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ReadmeContent holds the processed README ready for pushing to registries.
type ReadmeContent struct {
	Short string // max 100 chars
	Full  string // full markdown
}

// PrepareReadmeFromFile loads a README file and returns processed content ready for registry sync.
// Takes individual fields for maximum flexibility — callers resolve config to args.
func PrepareReadmeFromFile(file, description, linkBase, rootDir string) (*ReadmeContent, error) {
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

	// Rewrite relative links if link_base is set
	if linkBase != "" {
		content = rewriteRelativeLinks(content, linkBase)
	}

	// Generate short description
	short := description
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

// DeriveRawBase auto-derives a raw file URL base from a link_base URL.
// Supports GitHub, GitLab, and Gitea URL patterns.
func DeriveRawBase(linkBase string) string {
	if linkBase == "" {
		return ""
	}

	// GitHub: github.com/{owner}/{repo}/blob/{branch} → raw.githubusercontent.com/{owner}/{repo}/{branch}
	if strings.Contains(linkBase, "github.com/") {
		s := strings.Replace(linkBase, "github.com/", "raw.githubusercontent.com/", 1)
		s = strings.Replace(s, "/blob/", "/", 1)
		return s
	}

	// GitLab: gitlab.com/{owner}/{repo}/-/blob/{branch} → gitlab.com/{owner}/{repo}/-/raw/{branch}
	if strings.Contains(linkBase, "/-/blob/") {
		return strings.Replace(linkBase, "/-/blob/", "/-/raw/", 1)
	}

	// Gitea: {host}/{owner}/{repo}/src/branch/{branch} → {host}/{owner}/{repo}/raw/branch/{branch}
	if strings.Contains(linkBase, "/src/branch/") {
		return strings.Replace(linkBase, "/src/branch/", "/raw/branch/", 1)
	}

	return ""
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
