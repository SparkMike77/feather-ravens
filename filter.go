package main

import "strings"

// matchInterest is a deliberately simple first pass: case-insensitive substring match of any
// configured interest keyword against the story's title or summary. Returns the matched keyword
// (useful context to pass along downstream) and whether anything matched at all. No semantic/LLM
// matching here - that's a later refinement, not part of this scaffold - see the project README.
func matchInterest(story Story, interests []string) (keyword string, ok bool) {
	if len(interests) == 0 {
		return "", false
	}
	haystack := strings.ToLower(story.Title + " " + story.Summary)
	for _, interest := range interests {
		if strings.Contains(haystack, strings.ToLower(interest)) {
			return interest, true
		}
	}
	return "", false
}
