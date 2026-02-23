// Package gitver provides git-based version detection and tag template
// resolution. It is the shared foundation used by both the docker build
// pipeline and the release management system.
package gitver

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// VersionInfo holds resolved version metadata from git.
type VersionInfo struct {
	Version      string // full version: "1.2.3", "1.2.3-alpha.1", "0.0.0-dev+abc1234"
	Base         string // semver base without prerelease: "1.2.3"
	Major        string
	Minor        string
	Patch        string
	Prerelease   string // "alpha.1", "beta.2", "rc.1", or "" for stable
	SHA          string
	Branch       string
	IsRelease    bool // true if HEAD is exactly at a tag
	IsPrerelease bool // true if tag has a prerelease suffix
}

// semverRe captures major.minor.patch and optional -prerelease suffix.
var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-(.+))?$`)

// DetectVersion resolves version info from git tags and refs.
func DetectVersion(rootDir string) (*VersionInfo, error) {
	v := &VersionInfo{}

	// Get current SHA
	sha, err := gitCmd(rootDir, "rev-parse", "--short=7", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("getting HEAD SHA: %w", err)
	}
	v.SHA = sha

	// Get current branch
	branch, err := gitCmd(rootDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		v.Branch = branch
	}

	// Try to get version from git describe (nearest tag)
	desc, err := gitCmd(rootDir, "describe", "--tags", "--abbrev=0")
	if err != nil {
		// No tags — use dev version
		v.Version = fmt.Sprintf("0.0.0-dev+%s", v.SHA)
		v.Base = "0.0.0"
		v.Major = "0"
		v.Minor = "0"
		v.Patch = "0"
		return v, nil
	}

	// Check if HEAD is exactly the tag (clean release)
	exactTag, exactErr := gitCmd(rootDir, "describe", "--tags", "--exact-match")
	v.IsRelease = exactTag != "" && exactErr == nil

	// Parse semver from tag
	tag := strings.TrimSpace(desc)
	if m := semverRe.FindStringSubmatch(tag); m != nil {
		v.Major = m[1]
		v.Minor = m[2]
		v.Patch = m[3]
		v.Base = fmt.Sprintf("%s.%s.%s", m[1], m[2], m[3])

		if m[4] != "" {
			// Has prerelease suffix: 0.2.0-alpha.1
			v.Prerelease = m[4]
			v.IsPrerelease = true
			v.Version = fmt.Sprintf("%s-%s", v.Base, v.Prerelease)
		} else {
			v.Version = v.Base
		}
	} else {
		// Non-semver tag — use it raw
		raw := strings.TrimPrefix(tag, "v")
		v.Version = raw
		v.Base = raw
	}

	// If not a release, append dev suffix
	if !v.IsRelease {
		v.Version = fmt.Sprintf("%s-dev+%s", v.Version, v.SHA)
	}

	return v, nil
}

// gitCmd runs a git command and returns trimmed stdout.
func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
