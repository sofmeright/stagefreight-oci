// Package release handles release notes generation, release creation,
// and cross-platform sync.
package release

import (
	"fmt"
	"strings"
)

// BadgeSVG generates a shields.io-style SVG badge for the release version.
func BadgeSVG(version, status string) string {
	// Determine right-side color based on status
	var color string
	switch status {
	case "passed", "success":
		color = "#4c1" // green
	case "warning":
		color = "#dfb317" // yellow
	case "critical", "failed":
		color = "#e05d44" // red
	default:
		color = "#4c1"
	}

	label := "release"
	labelWidth := 50
	valueWidth := textWidth(version) + 10
	totalWidth := labelWidth + valueWidth

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20">`, totalWidth))
	b.WriteString(`<linearGradient id="b" x2="0" y2="100%">`)
	b.WriteString(`<stop offset="0" stop-color="#bbb" stop-opacity=".1"/>`)
	b.WriteString(`<stop offset="1" stop-opacity=".1"/>`)
	b.WriteString(`</linearGradient>`)
	b.WriteString(fmt.Sprintf(`<mask id="a"><rect width="%d" height="20" rx="3" fill="#fff"/></mask>`, totalWidth))
	b.WriteString(`<g mask="url(#a)">`)
	b.WriteString(fmt.Sprintf(`<rect width="%d" height="20" fill="#555"/>`, labelWidth))
	b.WriteString(fmt.Sprintf(`<rect x="%d" width="%d" height="20" fill="%s"/>`, labelWidth, valueWidth, color))
	b.WriteString(fmt.Sprintf(`<rect width="%d" height="20" fill="url(#b)"/>`, totalWidth))
	b.WriteString(`</g>`)
	b.WriteString(fmt.Sprintf(`<g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">`))
	b.WriteString(fmt.Sprintf(`<text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>`, labelWidth/2, label))
	b.WriteString(fmt.Sprintf(`<text x="%d" y="14">%s</text>`, labelWidth/2, label))
	b.WriteString(fmt.Sprintf(`<text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>`, labelWidth+valueWidth/2, version))
	b.WriteString(fmt.Sprintf(`<text x="%d" y="14">%s</text>`, labelWidth+valueWidth/2, version))
	b.WriteString(`</g>`)
	b.WriteString(`</svg>`)
	return b.String()
}

// textWidth estimates the pixel width of a string for badge rendering.
func textWidth(s string) int {
	// Rough estimate: ~6.5px per character for the font used
	return int(float64(len(s)) * 6.5)
}
