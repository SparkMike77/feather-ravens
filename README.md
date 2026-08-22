# feather-ravens

Small standalone Go daemons ("Ravens") that each poll one news source on a schedule, pull headline
summaries, and flag stories matching configured interests. One binary, one config file per source,
no shared state between instances.

Built as a data-collection front end for a personal AI assistant's proactive-monitoring layer, but
has no hard dependency on one — it just posts JSON to a configurable HTTP endpoint.

**Status: early scaffold.** Feed fetching and keyword matching work end to end. Full-article fetch,
LLM-based fact extraction, and the receiving HTTP endpoint don't exist yet (see `extract.go` /
`ingest.go`).

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

See `systemd/raven@.service` for running multiple sources as systemd units.

## License

MIT — see `LICENSE`.
