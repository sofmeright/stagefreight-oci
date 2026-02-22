package modules

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/sofmeright/stagefreight/src/lint"
)

func init() {
	lint.Register("tabs", func() lint.Module { return &tabsModule{} })
}

type tabsModule struct{}

func (m *tabsModule) Name() string        { return "tabs" }
func (m *tabsModule) DefaultEnabled() bool { return true }
func (m *tabsModule) AutoDetect() []string { return nil }

func (m *tabsModule) Check(ctx context.Context, file lint.FileInfo) ([]lint.Finding, error) {
	// Skip files where tabs are conventional
	if m.tabsExpected(file.Path) {
		return nil, nil
	}

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
		line := scanner.Text()

		// Only flag leading tabs (indentation), not tabs inside content
		if strings.HasPrefix(line, "\t") {
			findings = append(findings, lint.Finding{
				File:     file.Path,
				Line:     lineNum,
				Module:   m.Name(),
				Severity: lint.SeverityInfo,
				Message:  "tab indentation (spaces expected)",
			})
		}
	}

	return findings, scanner.Err()
}

// tabsExpected returns true for file types where tab indentation is conventional.
func (m *tabsModule) tabsExpected(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(path)

	// Go files use tabs by convention (gofmt)
	if ext == ".go" {
		return true
	}

	// Makefiles require tabs
	if base == "Makefile" || base == "makefile" || base == "GNUmakefile" || ext == ".mk" {
		return true
	}

	// .gitmodules, .gitconfig use tabs
	if base == ".gitmodules" || base == ".gitconfig" {
		return true
	}

	return false
}
