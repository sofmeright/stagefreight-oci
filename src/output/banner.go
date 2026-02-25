package output

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sofmeright/stagefreight/src/assets"
)

// BannerInfo holds the identity fields displayed alongside the logo.
type BannerInfo struct {
	Version string
	SHA     string
	Branch  string
	Date    string
}

// Banner prints the StageFreight logo banner with version info.
// Three rendering paths, all sharing the same side-by-side layout:
//   - Color + chafa available: runtime truecolor render via chafa
//   - Color, no chafa: prerendered 256-color art embedded at build time
//   - No-color: prerendered greyscale 256-color art
func Banner(w io.Writer, info BannerInfo, color bool) {
	var artLines []string
	if color {
		artLines = renderLogo()
		if artLines == nil {
			artLines = splitPrerendered(prerenderedColor)
		}
	} else {
		artLines = splitPrerendered(prerenderedGray)
	}

	textItems := buildIdentityText(info, color)
	printBanner(w, artLines, textItems)
}

// buildIdentityText assembles the identity lines shown beside the logo.
func buildIdentityText(info BannerInfo, color bool) []string {
	var items []string
	if color {
		items = append(items, "\033[1;36mStageFreight\033[0m")
		if info.Version != "" {
			items = append(items, "\033[36m"+info.Version+"\033[0m")
		}
		if info.SHA != "" && info.Branch != "" {
			items = append(items, "\033[36m"+info.SHA+" \033[0m· \033[36m"+info.Branch+"\033[0m")
		} else if info.SHA != "" {
			items = append(items, "\033[36m"+info.SHA+"\033[0m")
		}
		if info.Date != "" {
			items = append(items, "\033[36m"+info.Date+"\033[0m")
		}
	} else {
		items = append(items, "StageFreight")
		if info.Version != "" {
			items = append(items, info.Version)
		}
		if info.SHA != "" && info.Branch != "" {
			items = append(items, info.SHA+" · "+info.Branch)
		} else if info.SHA != "" {
			items = append(items, info.SHA)
		}
		if info.Date != "" {
			items = append(items, info.Date)
		}
	}
	return items
}

// printBanner composites art lines with identity text, vertically centered.
func printBanner(w io.Writer, artLines, textItems []string) {
	textLines := make([]string, len(artLines))
	startLine := (len(artLines) - len(textItems)) / 2
	for i, item := range textItems {
		idx := startLine + i
		if idx >= 0 && idx < len(textLines) {
			textLines[idx] = item
		}
	}

	fmt.Fprintln(w)
	for i, artLine := range artLines {
		if textLines[i] != "" {
			fmt.Fprintf(w, "%s   %s\n", artLine, textLines[i])
		} else {
			fmt.Fprintln(w, artLine)
		}
	}
	fmt.Fprintln(w)
}

// splitPrerendered splits a prerendered ANSI art constant into lines.
func splitPrerendered(art string) []string {
	return strings.Split(art, "\n")
}

// renderLogo writes the embedded PNG to a temp file and runs chafa.
// Returns nil if chafa is not available or fails.
func renderLogo() []string {
	chafaPath, err := exec.LookPath("chafa")
	if err != nil {
		return nil
	}

	tmp, err := os.CreateTemp("", "sf-logo-*.png")
	if err != nil {
		return nil
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(assets.LogoPNG); err != nil {
		tmp.Close()
		return nil
	}
	tmp.Close()

	cmd := exec.Command(chafaPath, "-s", "70x30", "--symbols", "block", "--work", "9", tmp.Name())
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	raw := strings.TrimRight(string(out), "\n")
	raw = strings.ReplaceAll(raw, "\033[?25l", "")
	raw = strings.ReplaceAll(raw, "\033[?25h", "")
	raw = strings.TrimRight(raw, "\n")

	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// NewBannerInfo creates a BannerInfo with today's date.
// Version, SHA, and Branch should be populated from gitver.VersionInfo.
func NewBannerInfo(version, sha, branch string) BannerInfo {
	return BannerInfo{
		Version: version,
		SHA:     sha,
		Branch:  branch,
		Date:    time.Now().UTC().Format("2006-01-02"),
	}
}
