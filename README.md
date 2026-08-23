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
