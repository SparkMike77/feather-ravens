package main

import "strings"

// matchesInterest is a deliberately simple first pass: case-insensitive substring match of any
// configured interest keyword against the story's title or summary. This is the cheap filter that
// keeps most stories from ever needing a full-article fetch; a semantic/LLM-assisted check for
// borderline cases is a later refinement, not part of this scaffold - see the project README.
func matchesInterest(story Story, interests []string) bool {
	if len(interests) == 0 {
		return false
	}
	haystack := strings.ToLower(story.Title + " " + story.Summary)
	for _, interest := range interests {
		if strings.Contains(haystack, strings.ToLower(interest)) {
			return true
		}
	}
	return false
}
