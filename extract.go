package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
)

const articleFetchTimeout = 20 * time.Second

// fetchArticleText fetches a matched story's full page and extracts the readable article text,
// stripping nav/ads/boilerplate. Deliberately deterministic - no LLM involved here; this is still
// collector work, not analysis (see candidate.go's comment).
func fetchArticleText(pageURL string) (string, error) {
	article, err := readability.FromURL(pageURL, articleFetchTimeout, func(r *http.Request) {
		r.Header.Set("User-Agent", userAgent)
	})
	if err != nil {
		return "", fmt.Errorf("extracting readable article: %w", err)
	}

	var text strings.Builder
	if err := article.RenderText(&text); err != nil {
		return "", fmt.Errorf("rendering article text: %w", err)
	}
	return text.String(), nil
}
