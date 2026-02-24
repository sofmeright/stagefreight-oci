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
// Color mode: renders the embedded logo via chafa at 35x15 with identity
// text floating beside it, vertically centered. Falls back to text-only
// if chafa is not available. No-color mode: plain text header.
func Banner(w io.Writer, info BannerInfo, color bool) {
	if !color {
		bannerPlainText(w, info)
		return
	}

	artLines := renderLogo()
	if artLines == nil {
		// chafa not available — text-only fallback with color
		bannerColorText(w, info)
		return
	}

	// Build the identity text lines to float beside the logo.
	var textItems []string
	textItems = append(textItems, "\033[1;36mStageFreight\033[0m")
	if info.Version != "" {
		textItems = append(textItems, "\033[36m"+info.Version+"\033[0m")
	}
	if info.SHA != "" && info.Branch != "" {
		textItems = append(textItems, "\033[36m"+info.SHA+" \033[0m· \033[36m"+info.Branch+"\033[0m")
	} else if info.SHA != "" {
		textItems = append(textItems, "\033[36m"+info.SHA+"\033[0m")
	}
	if info.Date != "" {
		textItems = append(textItems, "\033[36m"+info.Date+"\033[0m")
	}

	// Vertically center text items against the art.
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

	cmd := exec.Command(chafaPath, "-s", "35x15", tmp.Name())
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	raw := strings.TrimRight(string(out), "\n")
	// Strip chafa's cursor-hide/show sequences
	raw = strings.ReplaceAll(raw, "\033[?25l", "")
	raw = strings.ReplaceAll(raw, "\033[?25h", "")
	raw = strings.TrimRight(raw, "\n")

	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// bannerPlainText prints a minimal text-only banner for no-color mode.
func bannerPlainText(w io.Writer, info BannerInfo) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "    StageFreight %s\n", info.Version)
	if info.SHA != "" && info.Branch != "" {
		fmt.Fprintf(w, "    %s · %s\n", info.SHA, info.Branch)
	}
	fmt.Fprintln(w)
}

// bannerColorText prints a styled text-only banner when chafa is unavailable.
func bannerColorText(w io.Writer, info BannerInfo) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "    \033[1;36mStageFreight\033[0m \033[36m%s\033[0m\n", info.Version)
	if info.SHA != "" && info.Branch != "" {
		fmt.Fprintf(w, "    \033[36m%s\033[0m · \033[36m%s\033[0m\n", info.SHA, info.Branch)
	}
	fmt.Fprintln(w)
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
