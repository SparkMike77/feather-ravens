package main

import "time"

// Candidate is a story that matched an interest, with its full article text fetched. This - not
// an extracted "fact" - is what a Raven sends onward: turning raw article text into structured,
// atomic facts is an LLM-assisted step, which belongs to Feather's own analyzer, not to a
// deterministic collector like this one (see README's Collector/Analyzer split, and
// Projects/Feather/Proactive/README.md in the Obsidian vault for the fuller design).
type Candidate struct {
	Source          string    `json:"source"` // this Raven's configured name (cfg.Name)
	ArticleURL      string    `json:"article_url"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`          // original feed summary, kept for context
	FullText        string    `json:"full_text"`        // readability-extracted article body
	MatchedInterest string    `json:"matched_interest"` // which configured keyword triggered this
	PublishedAt     time.Time `json:"published_at"`     // the article's own publish date (freshness) - zero value if the feed had none
	FetchedAt       time.Time `json:"fetched_at"`       // when this Raven fetched it, distinct from when the article was published
}
