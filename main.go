// Command raven periodically polls one news source, pulls headline summaries, and - for stories
// matching configured interests - fetches the full article text and forwards it to Feather for
// analysis. One Raven process = one news source, driven entirely by its config file - see
// raven.example.toml. Run multiple instances (one config each) to cover multiple sources;
// systemd/raven@.service is a template for doing that as systemd units.
package main

import (
	"flag"
	"log"
	"time"
)

// Identifies this Raven to the sites/feeds it polls - polite practice, and some sites/anti-bot
// rules reject requests with no or a generic User-Agent outright.
const userAgent = "feather-ravens/0.1 (+https://github.com/SparkMike77/feather-ravens)"

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
		keyword, ok := matchInterest(story, cfg.Interests)
		if !ok {
			continue
		}
		log.Printf("raven: interesting story (matched %q): %q (%s)", keyword, story.Title, story.Link)

		fullText, err := fetchArticleText(story.Link)
		if err != nil {
			log.Printf("raven: full-article fetch failed for %s: %v", story.Link, err)
			continue
		}

		candidate := Candidate{
			Source:          cfg.Name,
			ArticleURL:      story.Link,
			Title:           story.Title,
			Summary:         story.Summary,
			FullText:        fullText,
			MatchedInterest: keyword,
			FetchedAt:       time.Now().UTC(),
		}

		if err := postCandidate(cfg.IngestURL, candidate); err != nil {
			log.Printf("raven: failed to post candidate for %s: %v", story.Link, err)
			continue
		}
		log.Printf("raven: posted candidate for %q (%d chars)", story.Title, len(fullText))
	}
}
