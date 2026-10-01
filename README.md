# feather-ravens

Small standalone Go daemons ("Ravens") that each poll one news source on a schedule, pull headline
summaries, and — for stories matching configured interests — fetch the full article text and post
it onward as a JSON "candidate". One binary, one config file per source, no shared state between
instances.

Built as a data-collection front end for a personal AI assistant's proactive-monitoring layer, but
has no hard dependency on one — it just posts JSON to a configurable HTTP endpoint. Deliberately
stays deterministic throughout (feed fetch, keyword match, readability-style article extraction) —
turning a candidate's raw text into structured facts is an LLM-assisted step that belongs on the
receiving end, not here.

**Status:** feed fetching, keyword matching, full-article extraction, and posting candidates all
work end to end (verified against a live feed and a live Feather ingest endpoint).

## rss-mcp

`rss-mcp/` is a second binary in this repo: Feather's RSS *reader* - a stdio MCP server exposing a
configured feed list (`list_feeds`, `refresh_all_feeds`, `add_feed`, `update_feed`, `remove_feed`,
`check_feed_health`) plus one-off `fetch_feed_entries` / `fetch_article_content`. A Go port of the
TypeScript [feather-rss-mcp](https://github.com/SparkMike77/feather-rss-mcp), tool-for-tool
compatible (same names, parameters, JSON shapes, `FEEDS_CONFIG` / `HEALTH_LOG` env vars and
`feeds.config.json` format), so it replaces it by changing only the `command` Feather launches.

Feeds are fetched exactly as the Node version did - `User-Agent: rss-parser`,
`Accept: application/rss+xml`, HTTP/1.1 only - because some publishers filter on it: CBC's CDN
drops unrecognised user agents, and Hacker News answers non-browser HTTP/2 clients with a 419.

```sh
go build -o rss-mcp ./rss-mcp
FEEDS_CONFIG=/var/lib/feather/feeds.config.json ./rss-mcp   # speaks MCP on stdin/stdout
```

## Releases

Pushing a `v*` tag runs `.github/workflows/release.yml`, which attaches static Linux binaries
(`raven-linux-{amd64,arm64}`, `rss-mcp-linux-{amd64,arm64}`) and a `SHA256SUMS` file to the
release. Servers install from those - no Go toolchain needed (see Feather's
`deploy/setup-extras.sh`).

## Build

```sh
go build -o raven .
```

Cross-compile for Debian/amd64 from anywhere:

```sh
GOOS=linux GOARCH=amd64 go build -o raven .
```

## Configure & run

Copy `raven.example.toml` to `raven.toml`, edit it for your source, then:

```sh
./raven -config raven.toml
```

```toml
name = "BBC World News"
feed_url = "http://feeds.bbci.co.uk/news/world/rss.xml"
check_interval = "30m"
interests = ["climate", "elections", "central bank"]
ingest_url = "http://localhost:8765/proactive/ingest/news"
```

See `systemd/raven@.service` for running multiple sources as systemd units, or run
`sudo scripts/setup-raven.sh` to install one: prompts for a topic and a target feed (or take them
as `--topic`/`--name`/`--feed-url`/`--interval`/`--ingest-url` flags for non-interactive use - see
`setup-raven.sh --help`) and takes care of the build, config, unit install, and
`systemctl enable --now` in one pass. This is the one place that logic lives - both
[web/index.html](web/index.html)'s command builder and Feather's own `stage_raven_config` tool
generate a call to this same script rather than each re-implementing the install steps.

For anyone who'd rather not run an unfamiliar script blind, [web/index.html](web/index.html) is a
static, no-backend page that builds the same `raven.toml` interactively, generates the
`setup-raven.sh` command to run it, and explains how the whole thing works in plain language - see
[web/README.md](web/README.md). It's display-only: it generates text for you to run yourself over
SSH, never touches the server.

See [FUNCTIONS.md](FUNCTIONS.md) for a Mermaid diagram of every function's inputs/outputs.

## What gets posted

One `POST <ingest_url>` per matched story, JSON body:

```json
{
  "source": "BBC World News",
  "article_url": "https://...",
  "title": "...",
  "summary": "...",
  "full_text": "...",
  "matched_interest": "climate",
  "published_at": "2026-08-22T09:15:00Z",
  "fetched_at": "2026-08-22T12:00:00Z"
}
```

`published_at` is the article's own publish date (per the feed, zero value if it didn't provide
one) - the freshness signal for whatever facts get derived from this candidate later. Distinct from
`fetched_at`, which is just when this Raven happened to grab it.

Any non-2xx response is logged and skipped — a Raven never crashes or retries indefinitely over a
single delivery failure.

## License

MIT — see `LICENSE`.
