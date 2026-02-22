// Package release handles release notes generation, release creation,
// and cross-platform sync.
package release

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// CommitCategory represents a group of commits by type.
type CommitCategory struct {
	Title   string // display title (e.g., "Features", "Bug Fixes")
	Prefix  string // conventional commit prefix (e.g., "feat", "fix")
	Commits []Commit
}

// Commit is a parsed conventional commit.
type Commit struct {
	Hash    string
	Type    string // feat, fix, chore, etc.
	Scope   string // optional scope in parens
	Summary string
	Body    string
	Breaking bool
}

var conventionalRe = regexp.MustCompile(`^(\w+)(?:\(([^)]+)\))?(!)?\s*:\s*(.+)`)

// categoryOrder defines the display order for release notes.
var categoryOrder = []struct {
	prefix string
	title  string
}{
	{"BREAKING", "Breaking Changes"},
	{"feat", "Features"},
	{"fix", "Bug Fixes"},
	{"perf", "Performance"},
	{"security", "Security"},
	{"refactor", "Refactoring"},
	{"docs", "Documentation"},
	{"test", "Tests"},
	{"ci", "CI/CD"},
	{"chore", "Maintenance"},
	{"style", "Style"},
	{"migration", "Migrations"},
	{"hotfix", "Hotfixes"},
}

// GenerateNotes produces markdown release notes from git log between two refs.
// If fromRef is empty, it finds the previous tag automatically.
func GenerateNotes(repoDir, fromRef, toRef string, securitySummary string) (string, error) {
	if toRef == "" {
		toRef = "HEAD"
	}

	// Find previous tag if not specified
	if fromRef == "" {
		prev, err := previousTag(repoDir, toRef)
		if err != nil || prev == "" {
			// No previous tag — use all commits
			fromRef = ""
		} else {
			fromRef = prev
		}
	}

	// Get commits
	commits, err := parseCommits(repoDir, fromRef, toRef)
	if err != nil {
		return "", err
	}

	// Categorize
	categories := categorize(commits)

	// Render markdown
	return renderNotes(categories, securitySummary), nil
}

func previousTag(repoDir, currentRef string) (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0", currentRef+"^")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func parseCommits(repoDir, fromRef, toRef string) ([]Commit, error) {
	var rangeSpec string
	if fromRef != "" {
		rangeSpec = fromRef + ".." + toRef
	} else {
		rangeSpec = toRef
	}

	// Format: hash<SEP>subject<SEP>body
	cmd := exec.Command("git", "log", rangeSpec, "--format=%H\x1f%s\x1f%b\x1e")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var commits []Commit
	entries := strings.Split(string(out), "\x1e")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		fields := strings.SplitN(entry, "\x1f", 3)
		if len(fields) < 2 {
			continue
		}

		c := Commit{
			Hash:    fields[0][:7], // short hash
			Summary: fields[1],
		}
		if len(fields) > 2 {
			c.Body = strings.TrimSpace(fields[2])
		}

		// Parse conventional commit
		if m := conventionalRe.FindStringSubmatch(c.Summary); m != nil {
			c.Type = strings.ToLower(m[1])
			c.Scope = m[2]
			c.Breaking = m[3] == "!" || strings.Contains(strings.ToUpper(c.Body), "BREAKING CHANGE")
			c.Summary = m[4]
		}

		// Detect breaking change from body even without prefix
		if strings.Contains(strings.ToUpper(c.Body), "BREAKING CHANGE") {
			c.Breaking = true
		}

		commits = append(commits, c)
	}

	return commits, nil
}

func categorize(commits []Commit) []CommitCategory {
	buckets := make(map[string][]Commit)
	for _, c := range commits {
		key := c.Type
		if c.Breaking {
			key = "BREAKING"
		}
		if key == "" {
			key = "other"
		}
		buckets[key] = append(buckets[key], c)
	}

	var categories []CommitCategory
	for _, cat := range categoryOrder {
		if cs, ok := buckets[cat.prefix]; ok {
			categories = append(categories, CommitCategory{
				Title:   cat.title,
				Prefix:  cat.prefix,
				Commits: cs,
			})
			delete(buckets, cat.prefix)
		}
	}

	// Any remaining uncategorized commits
	var otherCommits []Commit
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		otherCommits = append(otherCommits, buckets[k]...)
	}
	if len(otherCommits) > 0 {
		categories = append(categories, CommitCategory{
			Title:   "Other Changes",
			Prefix:  "other",
			Commits: otherCommits,
		})
	}

	return categories
}

func renderNotes(categories []CommitCategory, securitySummary string) string {
	var b strings.Builder

	for _, cat := range categories {
		b.WriteString(fmt.Sprintf("### %s\n\n", cat.Title))
		for _, c := range cat.Commits {
			scope := ""
			if c.Scope != "" {
				scope = fmt.Sprintf("**%s**: ", c.Scope)
			}
			b.WriteString(fmt.Sprintf("- %s%s (%s)\n", scope, c.Summary, c.Hash))
		}
		b.WriteString("\n")
	}

	// Embed security summary if provided
	if securitySummary != "" {
		b.WriteString(securitySummary)
		b.WriteString("\n")
	}

	return b.String()
}
