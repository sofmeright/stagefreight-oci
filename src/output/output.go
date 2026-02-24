package output

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/sofmeright/stagefreight/src/lint"
)

// Colors for terminal output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

// Printer formats and writes lint findings.
type Printer struct {
	Writer io.Writer
	Color  bool
}

// NewPrinter creates a printer writing to stdout with color auto-detection.
func NewPrinter() *Printer {
	return &Printer{
		Writer: os.Stdout,
		Color:  isTerminal(),
	}
}

// Print outputs findings grouped by file, returns true if any critical findings exist.
func (p *Printer) Print(findings []lint.Finding) bool {
	if len(findings) == 0 {
		return false
	}

	// Group by file
	grouped := make(map[string][]lint.Finding)
	for _, f := range findings {
		grouped[f.File] = append(grouped[f.File], f)
	}

	// Sort files
	files := make([]string, 0, len(grouped))
	for f := range grouped {
		files = append(files, f)
	}
	sort.Strings(files)

	hasCritical := false

	for _, file := range files {
		ff := grouped[file]

		// Sort by line number within file
		sort.Slice(ff, func(i, j int) bool {
			if ff[i].Line != ff[j].Line {
				return ff[i].Line < ff[j].Line
			}
			return ff[i].Column < ff[j].Column
		})

		fmt.Fprintf(p.Writer, "\n%s\n", p.colorize(file, colorBold))

		for _, f := range ff {
			sev := p.severityStr(f.Severity)
			if f.Severity == lint.SeverityCritical {
				hasCritical = true
			}

			loc := fmt.Sprintf("%d", f.Line)
			if f.Column > 0 {
				loc = fmt.Sprintf("%d:%d", f.Line, f.Column)
			}

			fmt.Fprintf(p.Writer, "  %s %s %s %s\n",
				p.colorize(loc, colorGray),
				sev,
				p.colorize(f.Module, colorCyan),
				f.Message,
			)
		}
	}

	return hasCritical
}

// Summary prints a final summary line.
func (p *Printer) Summary(total, critical, warning, info int, filesScanned int) {
	parts := []string{}
	if critical > 0 {
		parts = append(parts, p.colorize(fmt.Sprintf("%d critical", critical), colorRed))
	}
	if warning > 0 {
		parts = append(parts, p.colorize(fmt.Sprintf("%d warning", warning), colorYellow))
	}
	if info > 0 {
		parts = append(parts, fmt.Sprintf("%d info", info))
	}

	summary := "no findings"
	if len(parts) > 0 {
		summary = strings.Join(parts, ", ")
	}

	fmt.Fprintf(p.Writer, "\n%s findings in %d files: %s\n",
		p.colorize(fmt.Sprintf("%d", total), colorBold),
		filesScanned,
		summary,
	)
}

func (p *Printer) severityStr(s lint.Severity) string {
	switch s {
	case lint.SeverityCritical:
		return p.colorize("CRIT", colorRed)
	case lint.SeverityWarning:
		return p.colorize("WARN", colorYellow)
	case lint.SeverityInfo:
		return p.colorize("INFO", colorGray)
	default:
		return s.String()
	}
}

func (p *Printer) colorize(text, color string) string {
	if !p.Color {
		return text
	}
	return color + text + colorReset
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// UseColor returns true if colored output should be used.
// Respects NO_COLOR env, TERM=dumb, and terminal detection.
func UseColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTerminal() || IsCI()
}

// LintTable writes a per-module stats table inside a section.
func LintTable(w io.Writer, stats []lint.ModuleStats, _ bool) {
	// Header
	fmt.Fprintf(w, "    │ %-16s%6s  %6s  %s\n", "module", "files", "cached", "findings")

	for _, s := range stats {
		fmt.Fprintf(w, "    │ %-16s%5d   %5d   %5d\n", s.Name, s.Files, s.Cached, s.Findings)
	}
}
