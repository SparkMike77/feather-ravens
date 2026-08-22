package main

// Fact is the placeholder shape for one atomic, structured claim pulled from a matched article.
// Expect this to change once the ingest contract is finalized together with the Feather-side
// endpoint that will receive it.
type Fact struct {
	Source     string `json:"source"`
	ArticleURL string `json:"article_url"`
	Title      string `json:"title"`
	FactText   string `json:"fact_text"`
	RawSummary string `json:"raw_summary"`
}

// extractFacts will fetch a matched story's full article and ask a local LLM (Ollama) to pull out
// discrete facts. Not implemented yet: both the full-article text-extraction approach (stripping
// nav/ads/boilerplate) and the Fact schema above are still being designed on the Feather side -
// see Projects/Feather/Proactive/README.md in the Obsidian vault for the current state of that.
func extractFacts(story Story) ([]Fact, error) {
	panic("extractFacts: not implemented yet - see this file's comment")
}
