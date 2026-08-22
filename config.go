package main

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is one Raven's full configuration - one TOML file per news source (see
// raven.example.toml). CheckInterval is parsed separately from CheckIntervalRaw because TOML has
// no native duration type; a plain string field plus time.ParseDuration is simpler and more
// obvious to read than a custom (un)marshaler.
type Config struct {
	Name             string   `toml:"name"`
	FeedURL          string   `toml:"feed_url"`
	CheckIntervalRaw string   `toml:"check_interval"`
	Interests        []string `toml:"interests"`
	IngestURL        string   `toml:"ingest_url"`

	CheckInterval time.Duration `toml:"-"`
}

func loadConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}

	raw := cfg.CheckIntervalRaw
	if raw == "" {
		raw = "30m" // sane default if the config omits it
	}
	interval, err := time.ParseDuration(raw)
	if err != nil {
		return nil, fmt.Errorf("bad check_interval %q: %w", raw, err)
	}
	cfg.CheckInterval = interval

	return &cfg, nil
}
