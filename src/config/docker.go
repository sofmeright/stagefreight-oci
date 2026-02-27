package config

// CacheConfig holds build cache settings.
type CacheConfig struct {
	Watch      []WatchRule `yaml:"watch"`
	AutoDetect *bool       `yaml:"auto_detect"`
}

// WatchRule defines a cache-busting rule.
type WatchRule struct {
	Paths       []string `yaml:"paths"`
	Invalidates []string `yaml:"invalidates"`
}
