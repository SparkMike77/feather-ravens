# Function diagrams

One diagram per function in this repo: what you need to give it, and what comes back out. Meant to
be readable without knowing Go - see [main.go](main.go) etc. for the actual implementation. Mirrored
on the Obsidian vault's root-level "Functional Diagrams" page.

## main

Entry point. Reads the `-config` flag, loads it, then checks the feed forever on the configured
interval - this is the process that keeps running once started.

```mermaid
flowchart TD
    A["command-line flag\n-config path/to/raven.toml"] --> F[["main"]]
    F --> L[loadConfig]
    L --> T["a timer on the config's\ncheck interval"]
    T -->|"immediately, then every tick"| R[runOnce]
    F --> B["nothing returned -\nruns forever as a long-lived process,\nlogging progress as it goes"]
```

## runOnce

One pass over the feed: fetch the headlines, and for every one that matches an interest, fetch the
full article and send it onward.

```mermaid
flowchart TD
    A["loaded Config"] --> F[["runOnce"]]
    F --> S1[fetchStories]
    S1 --> S2{"matchInterest\nfor each story"}
    S2 -->|matched| S3[fetchArticleText]
    S2 -->|"no match"| SKIP["skipped"]
    S3 --> S4[postCandidate]
    F --> B["nothing returned -\nlogs each story it matches,\nposts, or fails on"]
```

## loadConfig

Reads one Raven's TOML config file and turns it into the settings the rest of the program uses.

```mermaid
flowchart LR
    A["path to a config file\n(e.g. raven.toml)"] --> F[["loadConfig"]]
    F --> B["a Config:\nname, feed URL, check interval,\ninterest keywords, ingest URL"]
    F -.->|"file missing or malformed"| E["error"]
```

## fetchStories

Downloads and parses the configured news feed into a plain list of headlines.

```mermaid
flowchart LR
    A["feed URL\n(from Config)"] --> F[["fetchStories"]]
    F <-->|"downloads and parses"| N[("the news feed itself")]
    F --> B["a list of stories:\ntitle, summary, link,\nand published date for each"]
    F -.->|"feed unreachable\nor unparseable"| E["error"]
```

## matchInterest

Checks one story's title/summary against the configured interest keywords.

```mermaid
flowchart LR
    A["one story\n(title + summary)"] --> F[["matchInterest"]]
    C["interest keywords\n(from Config)"] --> F
    F --> B{"did any keyword\nappear?"}
    B -->|yes| Y["which keyword matched"]
    B -->|no| N["no match"]
```

## fetchArticleText

Fetches a matched story's full web page and strips it down to just the readable article text - no
ads, navigation, or other page clutter.

```mermaid
flowchart LR
    A["a story's article URL"] --> F[["fetchArticleText"]]
    F <-->|"downloads and extracts\nthe readable content"| W[("the article's web page")]
    F --> B["the article's plain text"]
    F -.->|"page unreachable or\nnothing extractable"| E["error"]
```

## postCandidate

Sends one matched, full-text story onward to Feather (or whatever ingest endpoint is configured) as
JSON.

```mermaid
flowchart LR
    A["ingest URL\n(from Config)"] --> F[["postCandidate"]]
    C["a candidate:\nsource, title, summary, full text,\nmatched keyword, published/fetched dates"] --> F
    F -->|"sends as a JSON POST request"| S[("the configured\ningest endpoint")]
    F --> B["success"]
    F -.->|"endpoint unreachable or\nrejected the request"| E["error"]
```
