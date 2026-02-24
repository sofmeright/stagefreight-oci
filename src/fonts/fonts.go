// Package fonts provides embedded TTF fonts shared across StageFreight packages.
package fonts

import (
	"embed"
	"sort"
)

//go:embed *.ttf
var FS embed.FS

// Builtin maps config names to embedded filenames.
var Builtin = map[string]string{
	"dejavu-sans":  "DejaVuSans.ttf",
	"vera":         "Vera.ttf",
	"monofur":      "Monofur.ttf",
	"vera-mono":    "VeraMono.ttf",
	"ethereal":     "Ethereal.ttf",
	"playpen-sans": "PlaypenSans.ttf",
	"doto":         "Doto.ttf",
	"penyae":       "Penyae.ttf",
}

// DefaultFont is the config name of the default built-in font.
const DefaultFont = "dejavu-sans"

// Names returns sorted list of available built-in font names.
func Names() []string {
	names := make([]string, 0, len(Builtin))
	for k := range Builtin {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
