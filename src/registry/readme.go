package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/narrator"
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

	// 3.5. Narrator: compose badges into managed sections
	if len(cfg.Badges) > 0 {
		linkBase := strings.TrimRight(cfg.LinkBase, "/")
		rawBase := cfg.RawBase
		if rawBase == "" {
			rawBase = DeriveRawBase(linkBase)
		}
		rawBase = strings.TrimRight(rawBase, "/")

		content = composeBadgeSections(content, cfg.Badges, linkBase, rawBase)
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

// composeBadgeSections groups badge entries by section, composes each group
// through the narrator, and injects the result into the corresponding
// <!-- sf:<section> --> markers. Single code path — no hand-rolled markdown.
func composeBadgeSections(content string, badges []config.BadgeEntry, linkBase, rawBase string) string {
	// Group badges by target section (default: "badges").
	groups := make(map[string][]narrator.Module)
	order := []string{} // preserve first-seen order
	for _, b := range badges {
		section := b.Section
		if section == "" {
			section = "badges"
		}
		mod := resolveBadgeModule(b, linkBase, rawBase)
		if mod == nil {
			continue
		}
		if _, seen := groups[section]; !seen {
			order = append(order, section)
		}
		groups[section] = append(groups[section], mod)
	}

	// Compose and inject each section.
	for _, section := range order {
		modules := groups[section]
		// All badges in a section on one row (future: config-driven row breaks).
		composed := narrator.Compose(modules)
		if composed == "" {
			continue
		}

		updated, found := ReplaceSection(content, section, composed)
		if found {
			content = updated
		} else {
			// No markers yet — insert wrapped section at the top.
			content = WrapSection(section, composed) + "\n\n" + content
		}
	}

	return content
}

// resolveBadgeModule resolves a BadgeEntry's URLs and returns a narrator Module.
// Returns nil if the entry can't be resolved (missing file and URL).
func resolveBadgeModule(b config.BadgeEntry, linkBase, rawBase string) narrator.Module {
	var imgURL string
	if b.URL != "" {
		imgURL = b.URL
	} else if b.File != "" && rawBase != "" {
		imgURL = rawBase + "/" + strings.TrimPrefix(b.File, "./")
	} else {
		return nil
	}

	link := b.Link
	if link != "" && !isAbsoluteURL(link) && linkBase != "" {
		link = linkBase + "/" + strings.TrimPrefix(link, "./")
	}

	return narrator.BadgeModule{
		Alt:    b.Alt,
		ImgURL: imgURL,
		Link:   link,
	}
}

// isAbsoluteURL checks if a URL is absolute (has scheme or starts with /).
func isAbsoluteURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "/")
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
