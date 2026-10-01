// Command rss-mcp is Feather's RSS reader MCP server: a stdio MCP server exposing the user's
// configured feed list (read/refresh/add/update/remove/health-check) plus one-off feed and article
// fetches.
//
// A Go port of the TypeScript feather-rss-mcp (github.com/SparkMike77/feather-rss-mcp, aee7ca2),
// kept tool-for-tool compatible - same tool names, parameter names, JSON result shapes, the
// FEEDS_CONFIG / HEALTH_LOG environment variables and the feeds.config.json format - so it drops
// into Feather's [servers.rss_reader] by changing only `command`. Living here, next to the
// ravens, means one Go toolchain (or one release download) covers both, and no Node.js on the
// server. One deliberate difference: fetch_article_content extracts with readability (the same
// extractor the ravens use) instead of the old hand-rolled selector heuristics.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// Entry summaries go straight into a local LLM's small context window - keep them short and
	// default to few entries so a routine feed check stays cheap.
	maxSummaryChars   = 200
	defaultEntryLimit = 5
	feedTimeout       = 30 * time.Second
	articleTimeout    = 30 * time.Second
	// Feed requests send exactly rss-parser's default headers (what the TypeScript version sent):
	// some publishers' bot filters only pass well-known feed-reader UAs - CBC's CDN silently
	// drops requests from an unrecognized one, while answering "rss-parser" normally.
	feedUserAgent = "rss-parser"
	feedAccept    = "application/rss+xml"
	// Article pages get a browser UA, as the TypeScript version did - many sites serve a bot UA a
	// stripped or blocked page.
	browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
)

// ---- feeds.config.json ------------------------------------------------------------------------

type FeedConfig struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url"`
	Limit *int   `json:"limit,omitempty"`
}

type FeedsConfig struct {
	Feeds []*FeedConfig `json:"feeds"`
}

// feedsConfigPath: FEEDS_CONFIG > feeds.config.json next to the binary > in the cwd; the first
// that exists, else the first candidate (a new file gets created there on first save).
func feedsConfigPath() string {
	if p := os.Getenv("FEEDS_CONFIG"); p != "" {
		abs, _ := filepath.Abs(p)
		return abs
	}
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "feeds.config.json"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "feeds.config.json"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

// configMu serializes read-modify-write of the config file across concurrent tool calls.
var configMu sync.Mutex

func loadFeedsConfig() *FeedsConfig {
	cfg := &FeedsConfig{}
	data, err := os.ReadFile(feedsConfigPath())
	if err != nil || json.Unmarshal(data, cfg) != nil {
		return &FeedsConfig{Feeds: []*FeedConfig{}}
	}
	if cfg.Feeds == nil {
		cfg.Feeds = []*FeedConfig{}
	}
	return cfg
}

func saveFeedsConfig(cfg *FeedsConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(feedsConfigPath(), append(data, '\n'), 0o644)
}

// ---- feed fetching ----------------------------------------------------------------------------

type RSSFeedEntry struct {
	Title      string   `json:"title"`
	Link       string   `json:"link"`
	PubDate    string   `json:"pubDate,omitempty"`
	Creator    string   `json:"creator,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Categories []string `json:"categories,omitempty"`
	GUID       string   `json:"guid,omitempty"`
}

type FeedInfo struct {
	Title         string         `json:"title"`
	Description   string         `json:"description,omitempty"`
	Link          string         `json:"link"`
	LastBuildDate string         `json:"lastBuildDate,omitempty"`
	Entries       []RSSFeedEntry `json:"entries"`
}

// feedHeaders adds the Accept header gofeed doesn't set itself.
type feedHeaders struct{ next http.RoundTripper }

func (f feedHeaders) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Header.Get("Accept") == "" {
		r = r.Clone(r.Context())
		r.Header.Set("Accept", feedAccept)
	}
	return f.next.RoundTrip(r)
}

// http1Transport: feeds are fetched over HTTP/1.1 only, like rss-parser (Node's http client)
// did. Some sites reject non-browser clients that negotiate HTTP/2 - Hacker News answers those
// with a 419 while serving the same request over HTTP/1.1 normally.
func http1Transport() http.RoundTripper {
	// Built from scratch rather than cloned from http.DefaultTransport: a clone can carry a TLS
	// config whose ALPN list already includes "h2", and then the server still picks HTTP/2.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:       &tls.Config{NextProtos: []string{"http/1.1"}},
		Protocols:             protocols,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

var (
	feedHTTPClient = &http.Client{Timeout: feedTimeout, Transport: feedHeaders{http1Transport()}}
	tagRE          = regexp.MustCompile(`<[^>]*>`)
	spaceRE        = regexp.MustCompile(`\s+`)
)

func parseFeed(ctx context.Context, feedURL string) (*gofeed.Feed, error) {
	parser := gofeed.NewParser()
	parser.Client = feedHTTPClient
	parser.UserAgent = feedUserAgent
	return parser.ParseURLWithContext(feedURL, ctx)
}

// plainSummary mirrors rss-parser's summary/contentSnippet: HTML stripped, entities decoded,
// whitespace collapsed, then truncated to maxSummaryChars.
func plainSummary(item *gofeed.Item) string {
	raw := item.Description
	if raw == "" {
		raw = item.Content
	}
	text := strings.TrimSpace(spaceRE.ReplaceAllString(html.UnescapeString(tagRE.ReplaceAllString(raw, " ")), " "))
	if r := []rune(text); len(r) > maxSummaryChars {
		return strings.TrimRight(string(r[:maxSummaryChars]), " \t\n") + "…"
	}
	return text
}

func toFeedInfo(feed *gofeed.Feed, fallbackURL, nameOverride string, limit int) FeedInfo {
	title := nameOverride
	if title == "" {
		title = feed.Title
	}
	if title == "" {
		title = "Untitled Feed"
	}
	link := feed.Link
	if link == "" {
		link = fallbackURL
	}
	info := FeedInfo{Title: title, Description: feed.Description, Link: link, LastBuildDate: feed.Updated, Entries: []RSSFeedEntry{}}
	for i, item := range feed.Items {
		if i >= limit {
			break
		}
		entry := RSSFeedEntry{
			Title:      item.Title,
			Link:       item.Link,
			PubDate:    item.Published,
			Summary:    plainSummary(item),
			Categories: item.Categories,
			GUID:       item.GUID,
		}
		if entry.Title == "" {
			entry.Title = "Untitled"
		}
		if entry.PubDate == "" {
			entry.PubDate = item.Updated
		}
		if len(item.Authors) > 0 && item.Authors[0] != nil {
			entry.Creator = item.Authors[0].Name
		}
		info.Entries = append(info.Entries, entry)
	}
	return info
}

// ---- health -----------------------------------------------------------------------------------

type FeedHealth struct {
	Name      string `json:"name,omitempty"`
	URL       string `json:"url"`
	Status    string `json:"status"` // Healthy | Unreachable | Error
	Detail    string `json:"detail"`
	CheckedAt string `json:"checkedAt"`
}

// classifyFeedError: network-level failures (DNS, refused/reset connections, timeouts) are
// Unreachable; a host that answered with a bad status or an unparseable body is an Error.
func classifyFeedError(err error) string {
	var dnsErr *net.DNSError
	var opErr *net.OpError
	var netErr net.Error
	switch {
	case errors.As(err, &dnsErr), errors.As(err, &opErr):
		return "Unreachable"
	case errors.As(err, &netErr) && netErr.Timeout(), errors.Is(err, context.DeadlineExceeded):
		return "Unreachable"
	}
	return "Error"
}

func healthLogPath() string {
	if p := os.Getenv("HEALTH_LOG"); p != "" {
		abs, _ := filepath.Abs(p)
		return abs
	}
	return filepath.Join(filepath.Dir(feedsConfigPath()), "rss-health.log")
}

var healthLogMu sync.Mutex

func logFeedHealth(h FeedHealth) {
	line, _ := json.Marshal(h)
	healthLogMu.Lock()
	defer healthLogMu.Unlock()
	f, err := os.OpenFile(healthLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("Failed to write health log: %v", err)
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}

func checkFeedHealth(ctx context.Context, fc FeedConfig) FeedHealth {
	h := FeedHealth{Name: fc.Name, URL: fc.URL, CheckedAt: nowISO()}
	feed, err := parseFeed(ctx, fc.URL)
	if err != nil {
		h.Status, h.Detail = classifyFeedError(err), err.Error()
	} else {
		h.Status, h.Detail = "Healthy", fmt.Sprintf("%d entries", len(feed.Items))
	}
	logFeedHealth(h)
	return h
}

// ---- helpers ----------------------------------------------------------------------------------

func nowISO() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil, nil
}

func validURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("Invalid URL format: %q", raw)
	}
	return nil
}

// validLimit accepts a JSON number that is an integer in 1..100 (models sometimes send 5.0).
func validLimit(limit *float64) (*int, error) {
	if limit == nil {
		return nil, nil
	}
	n := int(*limit)
	if float64(n) != *limit || n < 1 || n > 100 {
		return nil, fmt.Errorf("limit must be an integer between 1 and 100, got %v", *limit)
	}
	return &n, nil
}

// ---- tools ------------------------------------------------------------------------------------

type fetchFeedArgs struct {
	URL   string   `json:"url" jsonschema:"RSS/Atom feed URL"`
	Limit *float64 `json:"limit,omitempty" jsonschema:"max entries to return (1-100, default 5)"`
}

type urlArgs struct {
	URL string `json:"url" jsonschema:"URL"`
}

type optionalURLArgs struct {
	URL string `json:"url,omitempty" jsonschema:"a single feed URL to check; omit to check every configured feed"`
}

type noArgs struct{}

type addFeedArgs struct {
	URL   string   `json:"url" jsonschema:"feed URL"`
	Name  string   `json:"name,omitempty" jsonschema:"display name"`
	Limit *float64 `json:"limit,omitempty" jsonschema:"entries to show per refresh (1-100)"`
}

type updateFeedArgs struct {
	URL    string   `json:"url" jsonschema:"URL of the configured feed to update"`
	NewURL string   `json:"newUrl,omitempty" jsonschema:"replacement URL"`
	Name   *string  `json:"name,omitempty" jsonschema:"new display name"`
	Limit  *float64 `json:"limit,omitempty" jsonschema:"new entries-per-refresh limit (1-100)"`
}

func fetchFeedEntries(ctx context.Context, _ *mcp.CallToolRequest, a fetchFeedArgs) (*mcp.CallToolResult, any, error) {
	if err := validURL(a.URL); err != nil {
		return nil, nil, err
	}
	limit, err := validLimit(a.Limit)
	if err != nil {
		return nil, nil, err
	}
	n := defaultEntryLimit
	if limit != nil {
		n = *limit
	}
	feed, err := parseFeed(ctx, a.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to fetch RSS feed: %v", err)
	}
	return jsonResult(toFeedInfo(feed, a.URL, "", n))
}

type ArticleContent struct {
	Title       string `json:"title"`
	Content     string `json:"content"`
	URL         string `json:"url"`
	ExtractedAt string `json:"extractedAt"`
}

func fetchArticleContent(_ context.Context, _ *mcp.CallToolRequest, a urlArgs) (*mcp.CallToolResult, any, error) {
	if err := validURL(a.URL); err != nil {
		return nil, nil, err
	}
	article, err := readability.FromURL(a.URL, articleTimeout, func(r *http.Request) {
		r.Header.Set("User-Agent", browserUserAgent)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to fetch article content: %v", err)
	}
	var body strings.Builder
	if err := article.RenderHTML(&body); err != nil {
		return nil, nil, fmt.Errorf("Failed to fetch article content: %v", err)
	}
	markdown, err := htmltomarkdown.ConvertString(body.String())
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to fetch article content: %v", err)
	}
	markdown = strings.TrimSpace(regexp.MustCompile(`\n{3,}`).ReplaceAllString(markdown, "\n\n"))
	title := strings.TrimSpace(article.Title())
	if title == "" {
		title = "Untitled Article"
	}
	return jsonResult(ArticleContent{Title: title, Content: markdown, URL: a.URL, ExtractedAt: nowISO()})
}

func refreshAllFeeds(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	cfg := loadFeedsConfig()
	if len(cfg.Feeds) == 0 {
		return jsonResult(map[string]any{
			"refreshedAt": nowISO(), "feedCount": 0, "feeds": []any{},
			"message": "No feeds configured. Add entries to feeds.config.json.",
		})
	}
	results := make([]any, len(cfg.Feeds))
	var wg sync.WaitGroup
	for i, fc := range cfg.Feeds {
		wg.Add(1)
		go func(i int, fc FeedConfig) {
			defer wg.Done()
			feed, err := parseFeed(ctx, fc.URL)
			if err != nil {
				results[i] = map[string]string{"error": err.Error(), "url": fc.URL}
				return
			}
			n := defaultEntryLimit
			if fc.Limit != nil {
				n = *fc.Limit
			}
			results[i] = toFeedInfo(feed, fc.URL, fc.Name, n)
		}(i, *fc)
	}
	wg.Wait()
	return jsonResult(map[string]any{"refreshedAt": nowISO(), "feedCount": len(results), "feeds": results})
}

func listFeeds(_ context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	return jsonResult(loadFeedsConfig().Feeds)
}

func addFeed(_ context.Context, _ *mcp.CallToolRequest, a addFeedArgs) (*mcp.CallToolResult, any, error) {
	if err := validURL(a.URL); err != nil {
		return nil, nil, err
	}
	limit, err := validLimit(a.Limit)
	if err != nil {
		return nil, nil, err
	}
	configMu.Lock()
	defer configMu.Unlock()
	cfg := loadFeedsConfig()
	for _, f := range cfg.Feeds {
		if f.URL == a.URL {
			return nil, nil, fmt.Errorf("Feed already configured: %s", a.URL)
		}
	}
	entry := &FeedConfig{URL: a.URL, Name: a.Name, Limit: limit}
	cfg.Feeds = append(cfg.Feeds, entry)
	if err := saveFeedsConfig(cfg); err != nil {
		return nil, nil, fmt.Errorf("saving feeds config: %v", err)
	}
	return jsonResult(map[string]any{"added": entry, "feedCount": len(cfg.Feeds)})
}

func removeFeed(_ context.Context, _ *mcp.CallToolRequest, a urlArgs) (*mcp.CallToolResult, any, error) {
	if err := validURL(a.URL); err != nil {
		return nil, nil, err
	}
	configMu.Lock()
	defer configMu.Unlock()
	cfg := loadFeedsConfig()
	for i, f := range cfg.Feeds {
		if f.URL == a.URL {
			cfg.Feeds = append(cfg.Feeds[:i], cfg.Feeds[i+1:]...)
			if err := saveFeedsConfig(cfg); err != nil {
				return nil, nil, fmt.Errorf("saving feeds config: %v", err)
			}
			return jsonResult(map[string]any{"removed": f, "feedCount": len(cfg.Feeds)})
		}
	}
	return nil, nil, fmt.Errorf("No feed configured with URL: %s", a.URL)
}

func updateFeed(_ context.Context, _ *mcp.CallToolRequest, a updateFeedArgs) (*mcp.CallToolResult, any, error) {
	if err := validURL(a.URL); err != nil {
		return nil, nil, err
	}
	if a.NewURL == "" && a.Name == nil && a.Limit == nil {
		return nil, nil, errors.New("Provide at least one of newUrl, name, or limit to update.")
	}
	if a.NewURL != "" {
		if err := validURL(a.NewURL); err != nil {
			return nil, nil, err
		}
	}
	limit, err := validLimit(a.Limit)
	if err != nil {
		return nil, nil, err
	}
	configMu.Lock()
	defer configMu.Unlock()
	cfg := loadFeedsConfig()
	var feed *FeedConfig
	for _, f := range cfg.Feeds {
		if f.URL == a.URL {
			feed = f
		}
	}
	if feed == nil {
		return nil, nil, fmt.Errorf("No feed configured with URL: %s", a.URL)
	}
	if a.NewURL != "" {
		for _, f := range cfg.Feeds {
			if f != feed && f.URL == a.NewURL {
				return nil, nil, fmt.Errorf("Feed already configured: %s", a.NewURL)
			}
		}
		feed.URL = a.NewURL
	}
	if a.Name != nil {
		feed.Name = *a.Name
	}
	if limit != nil {
		feed.Limit = limit
	}
	if err := saveFeedsConfig(cfg); err != nil {
		return nil, nil, fmt.Errorf("saving feeds config: %v", err)
	}
	return jsonResult(map[string]any{"updated": feed, "feedCount": len(cfg.Feeds)})
}

func checkFeedHealthTool(ctx context.Context, _ *mcp.CallToolRequest, a optionalURLArgs) (*mcp.CallToolResult, any, error) {
	var targets []FeedConfig
	if a.URL != "" {
		if err := validURL(a.URL); err != nil {
			return nil, nil, err
		}
		targets = []FeedConfig{{URL: a.URL}}
	} else {
		for _, f := range loadFeedsConfig().Feeds {
			targets = append(targets, *f)
		}
	}
	results := make([]FeedHealth, len(targets))
	var wg sync.WaitGroup
	for i, fc := range targets {
		wg.Add(1)
		go func(i int, fc FeedConfig) {
			defer wg.Done()
			results[i] = checkFeedHealth(ctx, fc)
		}(i, fc)
	}
	wg.Wait()
	return jsonResult(map[string]any{"checkedAt": nowISO(), "feedCount": len(results), "results": results})
}

func boolPtr(b bool) *bool { return &b }

func main() {
	log.SetOutput(os.Stderr) // stdout is the MCP channel
	server := mcp.NewServer(&mcp.Implementation{Name: "feather-rss-mcp", Version: "2.0.0"}, nil)

	readOnlyOpen := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(true), IdempotentHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "fetch_feed_entries", Description: "Fetch RSS feed entries from a given URL",
		Annotations: readOnlyOpen}, fetchFeedEntries)
	mcp.AddTool(server, &mcp.Tool{Name: "fetch_article_content", Description: "Fetch and extract article content from a URL, formatted as Markdown",
		Annotations: readOnlyOpen}, fetchArticleContent)
	mcp.AddTool(server, &mcp.Tool{Name: "refresh_all_feeds", Description: "Refresh headlines from all RSS feeds configured in feeds.config.json (or FEEDS_CONFIG env var path)",
		Annotations: readOnlyOpen}, refreshAllFeeds)
	mcp.AddTool(server, &mcp.Tool{Name: "list_feeds", Description: "List all configured RSS feed sources",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(false), IdempotentHint: true}}, listFeeds)
	mcp.AddTool(server, &mcp.Tool{Name: "add_feed", Description: "Add a new RSS feed source to the configured feed list",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: boolPtr(false)}}, addFeed)
	mcp.AddTool(server, &mcp.Tool{Name: "remove_feed", Description: "Remove an RSS feed source from the configured feed list",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: boolPtr(false)}}, removeFeed)
	mcp.AddTool(server, &mcp.Tool{Name: "update_feed", Description: "Update an existing RSS feed source's URL, name, or entry limit",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: boolPtr(false)}}, updateFeed)
	mcp.AddTool(server, &mcp.Tool{Name: "check_feed_health",
		Description: "Connectivity/health check for RSS feeds. Checks a single URL if given, otherwise checks every feed in feeds.config.json. Reports each as Healthy, Unreachable, or Error, and appends the results to the health log.",
		Annotations: readOnlyOpen}, checkFeedHealthTool)

	log.Printf("Feather RSS MCP server running on stdio")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("rss-mcp: %v", err)
	}
}
