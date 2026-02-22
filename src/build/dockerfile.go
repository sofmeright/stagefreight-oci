package build

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

var (
	// FROM [--platform=...] <image> [AS <name>]
	fromRe = regexp.MustCompile(`(?i)^FROM\s+(?:--platform=\S+\s+)?(\S+)(?:\s+AS\s+(\S+))?`)
	// ARG <name>[=<default>]
	argRe = regexp.MustCompile(`(?i)^ARG\s+(\S+?)(?:=.*)?$`)
	// EXPOSE <port>[/<proto>]
	exposeRe = regexp.MustCompile(`(?i)^EXPOSE\s+(.+)`)
	// HEALTHCHECK ...
	healthcheckRe = regexp.MustCompile(`(?i)^HEALTHCHECK\s+(.+)`)
)

// ParseDockerfile extracts stage, arg, expose, and healthcheck info from a Dockerfile.
// This is a regex-based parser — not a full AST. Sufficient for detection and planning.
func ParseDockerfile(path string) (*DockerfileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info := &DockerfileInfo{}
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if m := fromRe.FindStringSubmatch(line); m != nil {
			stage := Stage{
				BaseImage: m[1],
				Line:      lineNum,
			}
			if len(m) > 2 {
				stage.Name = m[2]
			}
			info.Stages = append(info.Stages, stage)
			continue
		}

		if m := argRe.FindStringSubmatch(line); m != nil {
			info.Args = append(info.Args, m[1])
			continue
		}

		if m := exposeRe.FindStringSubmatch(line); m != nil {
			// EXPOSE can list multiple ports on one line
			ports := strings.Fields(m[1])
			info.Expose = append(info.Expose, ports...)
			continue
		}

		if m := healthcheckRe.FindStringSubmatch(line); m != nil {
			if !strings.EqualFold(m[1], "NONE") {
				hc := m[1]
				info.Healthcheck = &hc
			}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return info, nil
}
