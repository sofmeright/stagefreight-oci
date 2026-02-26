//go:build !banner_art

package output

// BannerArtANSI is empty unless built with -tags banner_art.
// The generated constant lives in banner_art_gen.go (gitignored).
const BannerArtANSI = ""
