package gitver

import (
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ResolveTemplate expands template variables in a single string against
// version info and environment. Works on any part of an image reference —
// registry URL, path, tag, or a fully composed image name.
//
// Supported templates:
//
//	Simple variables:
//	  {version}          → "1.2.3" or "1.2.3-alpha.1" (full version)
//	  {base}             → "1.2.3" (semver base, no prerelease)
//	  {major}            → "1"
//	  {minor}            → "2"
//	  {patch}            → "3"
//	  {prerelease}       → "alpha.1" or "" (empty for stable)
//	  {branch}           → "main", "develop"
//
//	Width-controlled variables — {name:N} truncates or pads to N:
//	  {sha}              → "abc1234" (default 7)
//	  {sha:12}           → "abc1234def01" (first 12 chars)
//	  {sha:4}            → "abc1" (first 4 chars)
//
//	Counters — resolved by the channel/tag manager at bump time:
//	  {n}                → "42" (decimal counter, no padding)
//	  {n:5}              → "00042" (zero-padded to 5 digits)
//	  {hex:4}            → "002a" (sequential hex counter, padded to 4 chars)
//	  {hex:8}            → "0000002a" (same counter, wider pad)
//
//	Random generators — fresh value each resolution:
//	  {rand:6}           → "084721" (random digits, exactly N chars)
//	  {rand:4}           → "3819"
//	  {randhex:8}        → "a3f7c012" (random hex, exactly N chars)
//	  {randhex:4}        → "b7e2"
//
//	Environment variables:
//	  {env:VAR_NAME}     → value of environment variable
//
//	Scoped version variables — {field:SCOPE} resolves from SCOPE-v* tags:
//	  {version:component}  → "1.0.3" (from component-v1.0.3 tag)
//	  {base:component}     → "1.0.3"
//	  {major:component}    → "1"
//	  {minor:component}    → "0"
//	  {patch:component}    → "3"
//	  {prerelease:component} → "" or "beta.1"
//
//	Time variables:
//	  {date}               → "2026-02-24" (ISO date, UTC)
//	  {datetime}           → "2026-02-24T15:04:05Z" (RFC3339)
//	  {timestamp}          → "1740412800" (unix epoch)
//
//	CI context variables (portable across GitLab/GitHub/Jenkins/Bitbucket):
//	  {ci.pipeline}        → pipeline/run ID
//	  {ci.runner}          → runner/agent name
//	  {ci.job}             → job name
//	  {ci.url}             → link to pipeline/run
//
//	Literals pass through as-is:
//	  "latest"           → "latest"
//
// Templates compose freely in any position:
//
//	"{env:REGISTRY}/myorg/myapp:{version}"
//	"version-{base}.{env:BUILD_NUM}"
//	"{branch}-{sha:10}"
//	"dev-{n:5}"
//	"build-{randhex:8}"
//	"nightly-{hex:4}"
func ResolveTemplate(tmpl string, v *VersionInfo) string {
	return ResolveTemplateWithDir(tmpl, v, "")
}

// ResolveTemplateWithDir expands template variables with scoped version support.
// rootDir is needed to resolve scoped versions via git; pass "" to skip scoped resolution.
func ResolveTemplateWithDir(tmpl string, v *VersionInfo, rootDir string) string {
	if v == nil {
		return tmpl
	}

	s := tmpl

	// Resolve scoped version templates first: {version:SCOPE}, {base:SCOPE}, etc.
	// These need git access so they require rootDir.
	if rootDir != "" {
		s = resolveScopedVersions(s, rootDir)
	}

	// Resolve parameterized templates (they contain colons that
	// could collide with simpler replacements)
	s = resolveEnvVars(s)
	s = resolveSHA(s, v.SHA)
	s = resolveRandHex(s)
	s = resolveRand(s)
	// {n:W} and {hex:W} counters are resolved by `stagefreight tag` at
	// release time, not during image builds. ResolveTemplate preserves
	// them so the channel system can interpret the pattern.

	// Time templates
	s = resolveTime(s)

	// CI context templates
	s = resolveCIContext(s)

	// Simple replacements (unscoped — from the primary VersionInfo)
	s = strings.ReplaceAll(s, "{version}", v.Version)
	s = strings.ReplaceAll(s, "{base}", v.Base)
	s = strings.ReplaceAll(s, "{major}", v.Major)
	s = strings.ReplaceAll(s, "{minor}", v.Minor)
	s = strings.ReplaceAll(s, "{patch}", v.Patch)
	s = strings.ReplaceAll(s, "{prerelease}", v.Prerelease)
	s = strings.ReplaceAll(s, "{branch}", sanitizeTag(v.Branch))
	s = strings.ReplaceAll(s, "{sha}", truncate(v.SHA, 7))

	return s
}

// ResolveTags expands tag templates against version info.
func ResolveTags(templates []string, v *VersionInfo) []string {
	if v == nil {
		return templates
	}
	tags := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		tags = append(tags, ResolveTemplate(tmpl, v))
	}
	return tags
}

// resolveEnvVars replaces all {env:VAR_NAME} with the env var value.
func resolveEnvVars(s string) string {
	for {
		start := strings.Index(s, "{env:")
		if start == -1 {
			return s
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			return s
		}
		end += start
		varName := s[start+5 : end]
		val := os.Getenv(varName)
		s = s[:start] + val + s[end+1:]
	}
}

// resolveSHA replaces {sha:N} with the SHA truncated to N chars.
// Plain {sha} is handled separately by the simple replacement pass.
func resolveSHA(s string, sha string) string {
	for {
		start := strings.Index(s, "{sha:")
		if start == -1 {
			return s
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			return s
		}
		end += start
		widthStr := s[start+5 : end]
		// Support legacy {sha:.7} syntax
		widthStr = strings.TrimPrefix(widthStr, ".")
		width, err := strconv.Atoi(widthStr)
		if err != nil || width <= 0 {
			width = 7
		}
		s = s[:start] + truncate(sha, width) + s[end+1:]
	}
}

// resolveRand replaces {rand:N} with N random decimal digits.
func resolveRand(s string) string {
	for {
		start := strings.Index(s, "{rand:")
		if start == -1 {
			return s
		}
		// Make sure this isn't {randhex:
		if start+6 < len(s) && strings.HasPrefix(s[start:], "{randhex:") {
			// Skip past this — handled by resolveRandHex
			next := strings.Index(s[start+1:], "{rand:")
			if next == -1 {
				return s
			}
			// Try again from the next occurrence — but simpler to just
			// require resolveRandHex runs first (which it does).
			return s
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			return s
		}
		end += start
		widthStr := s[start+6 : end]
		width, err := strconv.Atoi(widthStr)
		if err != nil || width <= 0 {
			width = 6
		}
		s = s[:start] + randomDigits(width) + s[end+1:]
	}
}

// resolveRandHex replaces {randhex:N} with N random hex characters.
func resolveRandHex(s string) string {
	for {
		start := strings.Index(s, "{randhex:")
		if start == -1 {
			return s
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			return s
		}
		end += start
		widthStr := s[start+9 : end]
		width, err := strconv.Atoi(widthStr)
		if err != nil || width <= 0 {
			width = 8
		}
		s = s[:start] + randomHex(width) + s[end+1:]
	}
}

// truncate returns the first n characters of s, or s if shorter.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// randomDigits returns n cryptographically random decimal digits.
func randomDigits(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	digits := make([]byte, n)
	for i := range b {
		digits[i] = '0' + b[i]%10
	}
	return string(digits)
}

// randomHex returns n cryptographically random hex characters.
func randomHex(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	h := fmt.Sprintf("%x", b)
	return truncate(h, n)
}

// resolveScopedVersions resolves {field:SCOPE} patterns by detecting versions
// from scoped git tags (SCOPE-v* pattern).
//
// Supported fields: version, base, major, minor, patch, prerelease.
// Example: {version:component} → looks for component-v* tags → "1.0.3"
//
// Scoped versions are cached per scope within a single resolution pass,
// so multiple fields with the same scope only trigger one git call.
func resolveScopedVersions(s string, rootDir string) string {
	scopedFields := []string{"version", "base", "major", "minor", "patch", "prerelease"}
	cache := make(map[string]*VersionInfo)

	for _, field := range scopedFields {
		prefix := "{" + field + ":"
		for {
			start := strings.Index(s, prefix)
			if start == -1 {
				break
			}
			end := strings.Index(s[start:], "}")
			if end == -1 {
				break
			}
			end += start

			scope := s[start+len(prefix) : end]

			// Skip if this looks like an env var, sha width, or other known pattern
			// (those are handled by their own resolvers)
			if scope == "" {
				break
			}

			// Resolve scoped version (cached)
			sv, ok := cache[scope]
			if !ok {
				var err error
				sv, err = DetectScopedVersion(rootDir, scope)
				if err != nil {
					sv = &VersionInfo{Version: "?", Base: "?", Major: "?", Minor: "?", Patch: "?"}
				}
				cache[scope] = sv
			}

			var val string
			switch field {
			case "version":
				val = sv.Version
			case "base":
				val = sv.Base
			case "major":
				val = sv.Major
			case "minor":
				val = sv.Minor
			case "patch":
				val = sv.Patch
			case "prerelease":
				val = sv.Prerelease
			}

			s = s[:start] + val + s[end+1:]
		}
	}

	return s
}

// resolveTime replaces time-related templates.
func resolveTime(s string) string {
	now := time.Now().UTC()

	s = strings.ReplaceAll(s, "{date}", now.Format("2006-01-02"))
	s = strings.ReplaceAll(s, "{datetime}", now.Format(time.RFC3339))
	s = strings.ReplaceAll(s, "{timestamp}", strconv.FormatInt(now.Unix(), 10))

	return s
}

// resolveCIContext replaces CI context templates with values from environment.
// Supports GitLab CI, GitHub Actions, Jenkins, and Bitbucket Pipelines.
func resolveCIContext(s string) string {
	s = strings.ReplaceAll(s, "{ci.pipeline}", firstEnv(
		"CI_PIPELINE_ID",         // GitLab
		"GITHUB_RUN_ID",          // GitHub Actions
		"BUILD_NUMBER",           // Jenkins
		"BITBUCKET_BUILD_NUMBER", // Bitbucket
	))
	s = strings.ReplaceAll(s, "{ci.runner}", firstEnv(
		"CI_RUNNER_DESCRIPTION", // GitLab
		"RUNNER_NAME",           // GitHub Actions
		"NODE_NAME",             // Jenkins
	))
	s = strings.ReplaceAll(s, "{ci.job}", firstEnv(
		"CI_JOB_NAME",       // GitLab
		"GITHUB_JOB",        // GitHub Actions
		"JOB_NAME",          // Jenkins
		"BITBUCKET_STEP_ID", // Bitbucket
	))
	s = strings.ReplaceAll(s, "{ci.url}", firstEnv(
		"CI_PIPELINE_URL",                // GitLab
		"GITHUB_SERVER_URL",              // GitHub (needs composition, but best effort)
		"BUILD_URL",                      // Jenkins
		"BITBUCKET_PIPELINE_RESULT_URL",  // Bitbucket (non-standard, best effort)
	))

	return s
}

// firstEnv returns the value of the first non-empty environment variable.
func firstEnv(names ...string) string {
	for _, name := range names {
		if val := os.Getenv(name); val != "" {
			return val
		}
	}
	return ""
}

// sanitizeTag replaces characters not allowed in Docker tags.
func sanitizeTag(s string) string {
	r := strings.NewReplacer(
		"/", "-",
		" ", "-",
	)
	return r.Replace(s)
}
