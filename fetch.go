package main

import "github.com/mmcdole/gofeed"

// Story is one headline/summary/link triple pulled from a feed - the raw shortlist, before any
// interest filtering happens.
type Story struct {
	Title   string
	Summary string
	Link    string
}

// fetchStories pulls and parses the configured feed. gofeed is used rather than hand-rolling XML
// parsing against the stdlib encoding/xml package: real-world RSS/Atom feeds are messier than the
// spec (mixed versions, namespaces, CDATA quirks), and gofeed already handles both formats
// robustly - not worth re-solving here.
func fetchStories(feedURL string) ([]Story, error) {
	parser := gofeed.NewParser()
	parser.UserAgent = userAgent
	feed, err := parser.ParseURL(feedURL)
	if err != nil {
		return nil, err
	}

	stories := make([]Story, 0, len(feed.Items))
	for _, item := range feed.Items {
		stories = append(stories, Story{
			Title:   item.Title,
			Summary: item.Description,
			Link:    item.Link,
		})
	}
	return stories, nil
}
