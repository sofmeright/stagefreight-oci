package badge

// Engine generates SVG badges using a specific font.
type Engine struct {
	metrics *FontMetrics
}

// New creates a badge engine with the given font metrics.
func New(metrics *FontMetrics) *Engine {
	return &Engine{metrics: metrics}
}

// Badge defines the content and appearance of a single badge.
type Badge struct {
	Label string // left side text
	Value string // right side text
	Color string // hex color for right side (e.g. "#4c1")
}

// Generate produces a shields.io-compatible SVG badge string.
func (e *Engine) Generate(b Badge) string {
	return e.renderSVG(b)
}

// StatusColor maps a status keyword to a badge hex color.
func StatusColor(status string) string {
	switch status {
	case "passed", "success":
		return "#4c1"
	case "warning":
		return "#dfb317"
	case "critical", "failed":
		return "#e05d44"
	default:
		return "#4c1"
	}
}
