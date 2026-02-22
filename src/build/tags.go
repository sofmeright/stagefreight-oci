package build

import "strings"

// ResolveTags expands tag templates against version info.
//
// Supported templates:
//
//	{version}        → "1.2.3"
//	{major}          → "1"
//	{minor}          → "2"
//	{patch}          → "3"
//	{major}.{minor}  → "1.2"
//	{branch}         → "main"
//	{sha}            → "abc1234"  (short)
//	{sha:.7}         → "abc1234"  (explicit length, same as short)
//	latest           → "latest"   (literal passthrough)
func ResolveTags(templates []string, v *VersionInfo) []string {
	if v == nil {
		return templates
	}

	tags := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		tag := tmpl
		tag = strings.ReplaceAll(tag, "{version}", v.Version)
		tag = strings.ReplaceAll(tag, "{major}", v.Major)
		tag = strings.ReplaceAll(tag, "{minor}", v.Minor)
		tag = strings.ReplaceAll(tag, "{patch}", v.Patch)
		tag = strings.ReplaceAll(tag, "{branch}", sanitizeTag(v.Branch))
		tag = strings.ReplaceAll(tag, "{sha}", v.SHA)
		tag = strings.ReplaceAll(tag, "{sha:.7}", v.SHA)
		tags = append(tags, tag)
	}
	return tags
}

// sanitizeTag replaces characters not allowed in Docker tags.
func sanitizeTag(s string) string {
	r := strings.NewReplacer(
		"/", "-",
		" ", "-",
	)
	return r.Replace(s)
}
