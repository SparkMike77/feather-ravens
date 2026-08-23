# Command builder

A single static HTML file - no build step, no backend, no external dependencies. It only builds
text (a `raven.toml` and the shell commands to install it); it never talks to the server itself.
Real control over a Raven always happens over SSH, by hand - see the main [README](../README.md)
and [scripts/setup-raven.sh](../scripts/setup-raven.sh) for the non-interactive path this
complements.

## Opening it

Any of these work equally well, since the page has no server-side component:

```sh
# just open the file directly
open web/index.html          # macOS
xdg-open web/index.html      # Linux
start web/index.html         # Windows

# or serve the folder if you'd rather have a URL
python3 -m http.server -d web 8000
```
