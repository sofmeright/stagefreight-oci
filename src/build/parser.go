package build

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LayerEvent represents a completed build layer parsed from buildx output.
type LayerEvent struct {
	Stage       string        // "builder", "stage-1", "" (for internal steps)
	StageStep   string        // "1/7", "2/7", etc.
	Instruction string        // "FROM", "COPY", "RUN", "WORKDIR", "ENV", "ARG", "EXPOSE", "ADD"
	Detail      string        // instruction arguments (truncated)
	Cached      bool          // true if layer was a cache hit
	Duration    time.Duration // layer execution time (0 for cached layers)
	Image       string        // for FROM: the base image name (without digest)
}

// layerState tracks in-progress state for a single buildx step.
type layerState struct {
	stage       string
	stageStep   string
	instruction string
	detail      string
	cached      bool
	done        bool
	seconds     float64
	image       string
}

// Regex patterns for buildx --progress=plain output.
var (
	// #N [stage M/N] INSTRUCTION args...
	layerStartRe = regexp.MustCompile(`^#(\d+) \[([^\]]*?) (\d+/\d+)\] (\w+)\s*(.*)`)
	// #N [internal] load build definition from Dockerfile (skip internal steps)
	internalRe = regexp.MustCompile(`^#\d+ \[internal\]`)
	// #N CACHED
	cachedRe = regexp.MustCompile(`^#(\d+) CACHED`)
	// #N DONE 44.8s
	doneRe = regexp.MustCompile(`^#(\d+) DONE (\d+\.?\d*)s`)
	// exporting to image
	exportingRe = regexp.MustCompile(`^#\d+ exporting to`)
	// FROM image@sha256:... — extract image name
	fromImageRe = regexp.MustCompile(`FROM\s+(\S+?)(?:@sha256:[a-f0-9]+)?(?:\s+AS\s+\S+)?$`)
)

// ParseBuildxOutput parses captured buildx --progress=plain output into layer events.
// Only meaningful build layers are returned (FROM, COPY, RUN, etc.).
// Internal steps (load build definition, load .dockerignore, metadata) are filtered out.
func ParseBuildxOutput(output string) []LayerEvent {
	layers := make(map[int]*layerState)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip internal steps
		if internalRe.MatchString(line) {
			continue
		}

		// Layer start: #N [stage M/N] INSTRUCTION args
		if m := layerStartRe.FindStringSubmatch(line); m != nil {
			stepNum, _ := strconv.Atoi(m[1])
			stage := m[2]
			stageStep := m[3]
			instruction := m[4]
			detail := m[5]

			// Extract base image name for FROM instructions
			var image string
			if instruction == "FROM" {
				if fm := fromImageRe.FindStringSubmatch(instruction + " " + detail); fm != nil {
					image = fm[1]
				}
			}

			// Truncate long details
			if len(detail) > 60 {
				detail = detail[:57] + "..."
			}

			layers[stepNum] = &layerState{
				stage:       stage,
				stageStep:   stageStep,
				instruction: instruction,
				detail:      detail,
				image:       image,
			}
			continue
		}

		// CACHED: #N CACHED
		if m := cachedRe.FindStringSubmatch(line); m != nil {
			stepNum, _ := strconv.Atoi(m[1])
			if ls, ok := layers[stepNum]; ok {
				ls.cached = true
				ls.done = true
			}
			continue
		}

		// DONE: #N DONE Ns
		if m := doneRe.FindStringSubmatch(line); m != nil {
			stepNum, _ := strconv.Atoi(m[1])
			seconds, _ := strconv.ParseFloat(m[2], 64)
			if ls, ok := layers[stepNum]; ok {
				ls.seconds = seconds
				ls.done = true
			}
			continue
		}
	}

	// Collect completed layers in step order.
	var events []LayerEvent
	// Find max step number to iterate in order.
	maxStep := 0
	for k := range layers {
		if k > maxStep {
			maxStep = k
		}
	}
	for i := 0; i <= maxStep; i++ {
		ls, ok := layers[i]
		if !ok || !ls.done {
			continue
		}
		// Skip exporting/writing steps
		if ls.instruction == "" {
			continue
		}

		event := LayerEvent{
			Stage:       ls.stage,
			StageStep:   ls.stageStep,
			Instruction: ls.instruction,
			Detail:      ls.detail,
			Cached:      ls.cached,
			Image:       ls.image,
		}
		if ls.seconds > 0 {
			event.Duration = time.Duration(ls.seconds * float64(time.Second))
		}
		events = append(events, event)
	}

	return events
}

// FormatLayerTiming formats a layer's timing for display.
// Returns "cached" for cache hits, or the duration string for completed layers.
func FormatLayerTiming(e LayerEvent) string {
	if e.Cached {
		return "cached"
	}
	if e.Duration > 0 {
		return formatBuildDuration(e.Duration)
	}
	return ""
}

// formatBuildDuration formats a duration for build layer display.
func formatBuildDuration(d time.Duration) string {
	if d >= time.Minute {
		return strconv.FormatFloat(d.Minutes(), 'f', 1, 64) + "m"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}

// FormatLayerInstruction formats a layer event into a display string.
// For FROM instructions, shows the base image name.
// For other instructions, shows the instruction and truncated detail.
func FormatLayerInstruction(e LayerEvent) string {
	if e.Instruction == "FROM" && e.Image != "" {
		return e.Image
	}
	if e.Detail != "" {
		return e.Instruction + " " + e.Detail
	}
	return e.Instruction
}
