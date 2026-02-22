package modules

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"unicode"
	"unicode/utf8"

	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/lint"
)

func init() {
	lint.Register("unicode", func() lint.Module { return &unicodeModule{} })
}

type unicodeModule struct{}

func (m *unicodeModule) Name() string           { return "unicode" }
func (m *unicodeModule) DefaultEnabled() bool    { return true }
func (m *unicodeModule) AutoDetect() []string    { return nil }

func (m *unicodeModule) Check(ctx context.Context, file lint.FileInfo) ([]lint.Finding, error) {
	f, err := os.Open(file.AbsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []lint.Finding
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		if !utf8.Valid(line) {
			findings = append(findings, lint.Finding{
				File:     file.Path,
				Line:     lineNum,
				Module:   m.Name(),
				Severity: lint.SeverityWarning,
				Message:  "invalid UTF-8 encoding",
			})
			continue
		}

		col := 0
		for i := 0; i < len(line); {
			r, size := utf8.DecodeRune(line[i:])
			col++

			if msg := checkRune(r); msg != "" {
				findings = append(findings, lint.Finding{
					File:     file.Path,
					Line:     lineNum,
					Column:   col,
					Module:   m.Name(),
					Severity: severityForRune(r),
					Message:  fmt.Sprintf("%s (U+%04X)", msg, r),
				})
			}

			i += size
		}
	}

	return findings, scanner.Err()
}

func checkRune(r rune) string {
	// Bidi override characters — can change text rendering direction for attacks
	switch r {
	case '\u202A': // LEFT-TO-RIGHT EMBEDDING
		return "bidi override: left-to-right embedding"
	case '\u202B': // RIGHT-TO-LEFT EMBEDDING
		return "bidi override: right-to-left embedding"
	case '\u202C': // POP DIRECTIONAL FORMATTING
		return "bidi override: pop directional formatting"
	case '\u202D': // LEFT-TO-RIGHT OVERRIDE
		return "bidi override: left-to-right override"
	case '\u202E': // RIGHT-TO-LEFT OVERRIDE
		return "bidi override: right-to-left override"
	case '\u2066': // LEFT-TO-RIGHT ISOLATE
		return "bidi override: left-to-right isolate"
	case '\u2067': // RIGHT-TO-LEFT ISOLATE
		return "bidi override: right-to-left isolate"
	case '\u2068': // FIRST STRONG ISOLATE
		return "bidi override: first strong isolate"
	case '\u2069': // POP DIRECTIONAL ISOLATE
		return "bidi override: pop directional isolate"
	}

	// Zero-width characters — invisible, used for trojan source attacks
	switch r {
	case '\u200B': // ZERO WIDTH SPACE
		return "zero-width space"
	case '\u200C': // ZERO WIDTH NON-JOINER
		return "zero-width non-joiner"
	case '\u200D': // ZERO WIDTH JOINER
		return "zero-width joiner"
	case '\uFEFF': // ZERO WIDTH NO-BREAK SPACE (BOM mid-file)
		return "zero-width no-break space (unexpected BOM)"
	}

	// Other invisible/confusable characters
	switch r {
	case '\u00AD': // SOFT HYPHEN
		return "soft hyphen (invisible)"
	case '\u034F': // COMBINING GRAPHEME JOINER
		return "combining grapheme joiner"
	case '\u2060': // WORD JOINER
		return "word joiner (invisible)"
	case '\u2061', '\u2062', '\u2063', '\u2064': // FUNCTION APPLICATION, INVISIBLE TIMES/SEPARATOR/PLUS
		return "invisible math operator"
	case '\u180E': // MONGOLIAN VOWEL SEPARATOR
		return "mongolian vowel separator (invisible whitespace)"
	}

	// Confusable whitespace (looks like a space but isn't)
	switch r {
	case '\u00A0': // NO-BREAK SPACE
		return "non-breaking space"
	case '\u2000', '\u2001', '\u2002', '\u2003', '\u2004',
		'\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A':
		return "unusual whitespace character"
	case '\u205F': // MEDIUM MATHEMATICAL SPACE
		return "medium mathematical space"
	case '\u3000': // IDEOGRAPHIC SPACE
		return "ideographic space"
	}

	// Control characters (except normal whitespace: \t \n \r)
	if r != '\t' && r != '\n' && r != '\r' && unicode.IsControl(r) && r < 0x80 {
		return "ASCII control character"
	}

	// Tag characters (U+E0001-U+E007F) — used for language tagging, exploitable
	if r >= 0xE0001 && r <= 0xE007F {
		return "tag character (invisible)"
	}

	return ""
}

func severityForRune(r rune) lint.Severity {
	// Bidi overrides and zero-width chars are critical — trojan source attack vectors
	switch {
	case r >= '\u202A' && r <= '\u202E':
		return lint.SeverityCritical
	case r >= '\u2066' && r <= '\u2069':
		return lint.SeverityCritical
	case r == '\u200B' || r == '\u200C' || r == '\u200D' || r == '\uFEFF':
		return lint.SeverityCritical
	case r >= 0xE0001 && r <= 0xE007F:
		return lint.SeverityCritical
	default:
		return lint.SeverityWarning
	}
}
