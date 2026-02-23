package retention

// TemplatesToPatterns converts StageFreight tag templates into regex patterns
// suitable for config.MatchPatterns.
//
// Template variables like {sha:8}, {version}, {branch} are replaced with
// regex wildcards (.+) so the pattern matches any resolved value.
//
// Examples:
//
//	"dev-{sha:8}"    → "^dev-.+$"
//	"{version}"      → "^.+$"
//	"latest"         → "^latest$"
//	"!{branch}-rc"   → "!^.+-rc$"
func TemplatesToPatterns(templates []string) []string {
	if len(templates) == 0 {
		return nil
	}

	patterns := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		patterns = append(patterns, TemplateToPattern(tmpl))
	}
	return patterns
}

// TemplateToPattern converts a single tag template to a regex pattern.
func TemplateToPattern(tmpl string) string {
	// Preserve negation prefix
	prefix := ""
	s := tmpl
	if len(s) > 0 && s[0] == '!' {
		prefix = "!"
		s = s[1:]
	}

	// Replace all {name} and {name:param} with .+
	result := make([]byte, 0, len(s)*2)
	i := 0
	for i < len(s) {
		if s[i] == '{' {
			// Find matching close brace
			j := i + 1
			for j < len(s) && s[j] != '}' {
				j++
			}
			if j < len(s) {
				// Replace {…} with .+
				result = append(result, '.', '+')
				i = j + 1
				continue
			}
		}
		// Escape regex metacharacters in literal parts
		if isRegexMeta(s[i]) {
			result = append(result, '\\')
		}
		result = append(result, s[i])
		i++
	}

	return prefix + "^" + string(result) + "$"
}

func isRegexMeta(c byte) bool {
	switch c {
	case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '\\', '^', '$', '|':
		return true
	}
	return false
}
