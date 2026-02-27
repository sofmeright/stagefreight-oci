package registry

import (
	"regexp"
	"strings"
)

// Managed section markers follow the pattern:
//
//	<!-- sf:<name> -->
//	...managed content...
//	<!-- /sf:<name> -->
//
// Any StageFreight module can own a named section. The markers are the contract:
// everything between them is managed by that module and will be replaced on
// each run. Content outside markers is never touched.
//
// Sections can appear anywhere in a document — top, middle, inside tables,
// inline on a single line (if the content fits). The only requirement is that
// the start marker appears before the end marker.

// SectionStart returns the opening marker for a named managed section.
func SectionStart(name string) string {
	return "<!-- sf:" + name + " -->"
}

// SectionEnd returns the closing marker for a named managed section.
func SectionEnd(name string) string {
	return "<!-- /sf:" + name + " -->"
}

// WrapSection wraps content in named section markers (block mode with newlines).
func WrapSection(name, content string) string {
	return SectionStart(name) + "\n" + content + "\n" + SectionEnd(name)
}

// WrapSectionInline wraps content in named section markers (inline, no newlines).
func WrapSectionInline(name, content string) string {
	return SectionStart(name) + content + SectionEnd(name)
}

// ReplaceSection finds the markers for the named section and replaces the
// content between them. Markers themselves are preserved. Returns the updated
// content and whether the section was found.
//
// Whitespace-aware: if both markers are on the same line in the source
// (inline section), replacement is inserted without newline padding.
// If markers are on separate lines (block section), replacement gets
// newline padding for readability. The source document declares the intent
// by how the markers are placed.
//
// If the section is not found, content is returned unchanged with found=false
// so the caller can decide where to insert a new section.
func ReplaceSection(content, name, replacement string) (updated string, found bool) {
	start := SectionStart(name)
	end := SectionEnd(name)

	startIdx := strings.Index(content, start)
	if startIdx < 0 {
		return content, false
	}

	// Search for end marker only after the start marker.
	afterStart := startIdx + len(start)
	endRelative := strings.Index(content[afterStart:], end)
	if endRelative < 0 {
		return content, false
	}
	endIdx := afterStart + endRelative

	// Detect inline vs block: if the content between markers contains no
	// newlines (or is empty), the section is inline.
	between := content[afterStart:endIdx]
	inline := !strings.Contains(between, "\n")

	sep := "\n"
	if inline {
		sep = ""
	}

	var b strings.Builder
	b.WriteString(content[:startIdx])
	b.WriteString(start)
	b.WriteString(sep)
	b.WriteString(replacement)
	b.WriteString(sep)
	b.WriteString(end)
	b.WriteString(content[endIdx+len(end):])

	return b.String(), true
}

// ReplaceBetween finds arbitrary start/end markers and replaces the content
// between them. Markers themselves are preserved. Works like ReplaceSection
// but with user-specified markers instead of the standard sf:NAME pattern.
// Whitespace detection: if markers are inline (no newline between), no padding.
func ReplaceBetween(content, startMarker, endMarker, replacement string) (updated string, found bool) {
	startIdx := strings.Index(content, startMarker)
	if startIdx < 0 {
		return content, false
	}

	afterStart := startIdx + len(startMarker)
	endRelative := strings.Index(content[afterStart:], endMarker)
	if endRelative < 0 {
		return content, false
	}
	endIdx := afterStart + endRelative

	// Detect inline vs block from existing whitespace between markers.
	between := content[afterStart:endIdx]
	inline := !strings.Contains(between, "\n")

	sep := "\n"
	if inline {
		sep = ""
	}

	var b strings.Builder
	b.WriteString(content[:startIdx])
	b.WriteString(startMarker)
	b.WriteString(sep)
	b.WriteString(replacement)
	b.WriteString(sep)
	b.WriteString(endMarker)
	b.WriteString(content[endIdx+len(endMarker):])

	return b.String(), true
}

// HasSection reports whether content contains markers for the named section.
func HasSection(content, name string) bool {
	return strings.Contains(content, SectionStart(name)) &&
		strings.Contains(content, SectionEnd(name))
}

// SectionContent extracts the content between section markers.
// Returns the content and whether the section was found.
func SectionContent(content, name string) (string, bool) {
	start := SectionStart(name)
	end := SectionEnd(name)

	startIdx := strings.Index(content, start)
	if startIdx < 0 {
		return "", false
	}

	afterStart := startIdx + len(start)
	endRelative := strings.Index(content[afterStart:], end)
	if endRelative < 0 {
		return "", false
	}

	between := content[afterStart : afterStart+endRelative]
	return strings.TrimPrefix(strings.TrimSuffix(between, "\n"), "\n"), true
}

// --- Placement operations ---

// InsertAboveSection inserts text before the opening <!-- sf:<name> --> marker.
// Returns the updated content and whether the section was found.
func InsertAboveSection(content, name, text string) (string, bool) {
	start := SectionStart(name)
	idx := strings.Index(content, start)
	if idx < 0 {
		return content, false
	}
	return content[:idx] + text + "\n" + content[idx:], true
}

// InsertBelowSection inserts text after the closing <!-- /sf:<name> --> marker.
// Returns the updated content and whether the section was found.
func InsertBelowSection(content, name, text string) (string, bool) {
	end := SectionEnd(name)
	idx := strings.Index(content, end)
	if idx < 0 {
		return content, false
	}
	after := idx + len(end)
	return content[:after] + "\n" + text + content[after:], true
}

// InsertAboveMatch inserts text before the first line matching the regex pattern.
// Returns the updated content and whether a match was found.
func InsertAboveMatch(content, pattern, text string) (string, bool) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return content, false
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if re.MatchString(line) {
			before := strings.Join(lines[:i], "\n")
			after := strings.Join(lines[i:], "\n")
			if before != "" {
				return before + "\n" + text + "\n" + after, true
			}
			return text + "\n" + after, true
		}
	}
	return content, false
}

// InsertBelowMatch inserts text after the first line matching the regex pattern.
// Returns the updated content and whether a match was found.
func InsertBelowMatch(content, pattern, text string) (string, bool) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return content, false
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if re.MatchString(line) {
			before := strings.Join(lines[:i+1], "\n")
			after := strings.Join(lines[i+1:], "\n")
			if after != "" {
				return before + "\n" + text + "\n" + after, true
			}
			return before + "\n" + text, true
		}
	}
	return content, false
}

// ReplaceMatch replaces lines matching the regex pattern with the provided text.
// All contiguous matching lines are replaced as a block.
// Returns the updated content and whether a match was found.
func ReplaceMatch(content, pattern, text string) (string, bool) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return content, false
	}

	lines := strings.Split(content, "\n")
	var result []string
	matched := false
	replaced := false

	for _, line := range lines {
		if re.MatchString(line) {
			if !replaced {
				result = append(result, text)
				replaced = true
			}
			matched = true
			// Skip additional matching lines (replace all contiguous matches).
		} else {
			replaced = false
			result = append(result, line)
		}
	}

	if !matched {
		return content, false
	}
	return strings.Join(result, "\n"), true
}

// --- Scoped placement (section + match combined) ---

// InsertAboveMatchInSection finds a regex match within a named section and
// inserts text above the matched line. The section markers are preserved.
func InsertAboveMatchInSection(content, sectionName, pattern, text string) (string, bool) {
	sectionBody, found := SectionContent(content, sectionName)
	if !found {
		return content, false
	}

	updated, matched := InsertAboveMatch(sectionBody, pattern, text)
	if !matched {
		return content, false
	}

	result, _ := ReplaceSection(content, sectionName, updated)
	return result, true
}

// InsertBelowMatchInSection finds a regex match within a named section and
// inserts text below the matched line. The section markers are preserved.
func InsertBelowMatchInSection(content, sectionName, pattern, text string) (string, bool) {
	sectionBody, found := SectionContent(content, sectionName)
	if !found {
		return content, false
	}

	updated, matched := InsertBelowMatch(sectionBody, pattern, text)
	if !matched {
		return content, false
	}

	result, _ := ReplaceSection(content, sectionName, updated)
	return result, true
}

// ReplaceMatchInSection finds a regex match within a named section and
// replaces matched lines. The section markers are preserved.
func ReplaceMatchInSection(content, sectionName, pattern, text string) (string, bool) {
	sectionBody, found := SectionContent(content, sectionName)
	if !found {
		return content, false
	}

	updated, matched := ReplaceMatch(sectionBody, pattern, text)
	if !matched {
		return content, false
	}

	result, _ := ReplaceSection(content, sectionName, updated)
	return result, true
}

// --- High-level placement ---

// PlaceContent applies a placement operation to inject content into a document.
// Handles all combinations: section, match, section+match, top, bottom.
// The position parameter should already be normalized (above/below/replace/top/bottom).
// If inline is true, first-insertion wraps inline; if plain is true, no markers.
func PlaceContent(content, sectionName, position, matchPattern, text string, inline, plain bool) string {
	// Special document-level positions.
	if position == "top" && sectionName == "" && matchPattern == "" {
		if plain {
			return text + "\n" + content
		}
		if inline {
			return WrapSectionInline(sectionName, text) + "\n" + content
		}
		return WrapSection(sectionName, text) + "\n" + content
	}
	if position == "bottom" && sectionName == "" && matchPattern == "" {
		if plain {
			return content + "\n" + text
		}
		if inline {
			return content + "\n" + WrapSectionInline(sectionName, text)
		}
		return content + "\n" + WrapSection(sectionName, text)
	}

	// Section + Match combined (scoped regex).
	if sectionName != "" && matchPattern != "" {
		var result string
		var ok bool
		switch position {
		case "above":
			result, ok = InsertAboveMatchInSection(content, sectionName, matchPattern, text)
		case "replace":
			result, ok = ReplaceMatchInSection(content, sectionName, matchPattern, text)
		default: // "below"
			result, ok = InsertBelowMatchInSection(content, sectionName, matchPattern, text)
		}
		if ok {
			return result
		}
		return content
	}

	// Section-only placement.
	if sectionName != "" && matchPattern == "" {
		switch position {
		case "above":
			if result, ok := InsertAboveSection(content, sectionName, text); ok {
				return result
			}
		case "replace":
			if result, ok := ReplaceSection(content, sectionName, text); ok {
				return result
			}
		default: // "below"
			if result, ok := InsertBelowSection(content, sectionName, text); ok {
				return result
			}
		}
		return content
	}

	// Match-only placement (no section scope).
	if matchPattern != "" {
		var result string
		var ok bool
		switch position {
		case "above":
			result, ok = InsertAboveMatch(content, matchPattern, text)
		case "replace":
			result, ok = ReplaceMatch(content, matchPattern, text)
		default: // "below"
			result, ok = InsertBelowMatch(content, matchPattern, text)
		}
		if ok {
			return result
		}
		return content
	}

	// No placement specified — prepend to top (default).
	if plain {
		return text + "\n" + content
	}
	return content
}
