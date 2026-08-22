// Command raven periodically polls one news source, pulls headline summaries, and flags stories
// matching configured interests. One Raven process = one news source, driven entirely by its
// config file - see raven.example.toml. Run multiple instances (one config each) to cover
// multiple sources; systemd/raven@.service is a template for doing that as systemd units.
package main

import (
	"flag"
	"log"
	"time"
)

func main() {
	configPath := flag.String("config", "raven.toml", "path to this Raven's TOML config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("raven: failed to load config %q: %v", *configPath, err)
	}

	log.Printf("raven: watching %q (%s), checking every %s", cfg.Name, cfg.FeedURL, cfg.CheckInterval)

	ticker := time.NewTicker(cfg.CheckInterval)
	defer ticker.Stop()

	runOnce(cfg) // check immediately on startup rather than waiting a full interval first
	for range ticker.C {
		runOnce(cfg)
	}
}

func runOnce(cfg *Config) {
	stories, err := fetchStories(cfg.FeedURL)
	if err != nil {
		log.Printf("raven: fetch failed: %v", err)
		return
	}

	for _, story := range stories {
		if !matchesInterest(story, cfg.Interests) {
			continue
		}
		log.Printf("raven: interesting story: %q (%s)", story.Title, story.Link)
		// Full-article fetch + fact extraction + posting to Feather's ingest endpoint aren't
		// wired up yet - the text-extraction approach and the fact JSON schema/ingest contract
		// are still being designed on the Feather side. See extract.go and ingest.go.
	}
}
