# incplot-server

An HTTP server that exposes [incplot](https://github.com/InCom-0/incplot) as a web service, with built-in DuckDB data sources and multiple output formats. Distributed as a container image at `ghcr.io/mskyttner/incplot-server`.

## Quick start

```bash
podman run --rm -p 8080:8080 ghcr.io/mskyttner/incplot-server:latest
```

Then open `http://localhost:8080/incplot/ui` in a browser.

## Output formats

| `format=` | Content-Type | Use case |
|-----------|-------------|----------|
| `html` | `text/html` | Browser embed via `<iframe>` |
| `svg` | `image/svg+xml` | Scalable image (via ansitosvg) |
| `svg2` | `image/svg+xml` | Scalable image (via HTML→SVG transform) |
| `text` | `text/plain` | ANSI terminal output, Ghostty, MCP responses |

## API

### Sources

```
POST /incplot/sources          Register a DuckDB SQL data source
GET  /incplot/sources          List registered sources
GET  /incplot/source/{name}    Stream source data as NDJSON
GET  /incplot/data?sql=...     Ad-hoc DuckDB SQL as NDJSON
```

### Rendering

```
GET /incplot/plot?source=...&format=...&type=...&width=...&theme=...&font=...
```

| Parameter | Values | Default |
|-----------|--------|---------|
| `source` | URL or `/incplot/source/{name}` | — |
| `format` | `html`, `svg`, `svg2`, `text` | `html` |
| `type` | `line`, `scatter`, `barV`, `barHS`, `barHM`, `barVM` | auto |
| `width` | character columns | `80` |
| `theme` | `solarized_light`, `solarized_dark`, … | `solarized_light` |
| `font` | `Adwaita Mono`, `JetBrains Mono NF`, `unscii` | `Adwaita Mono` |

### Example — register and plot a sine wave

```bash
# Register a source
curl -X POST http://localhost:8080/incplot/sources \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "sine",
    "label": "Sine wave",
    "sql": "SELECT i AS x, round(sin(i * 0.3), 4) AS y FROM range(0, 40) t(i)"
  }'

# Fetch as ANSI (works in Ghostty and other true-colour terminals)
curl "http://localhost:8080/incplot/plot?source=/incplot/source/sine&format=text&width=80"
```

## Built-in sources

| Name | Description |
|------|-------------|
| `german_economy` | German economic indicators 1991–2023 |
| `euro_economies` | European economies dataset |
| `iris` | Iris dataset |
| `wine_quality` | Wine quality dataset |

## MCP integration

`format=text` returns stripped ANSI as UTF-8, suitable for embedding in a markdown code block in a TUI client (Claude Code, etc.). `format=svg2` returns a standalone SVG element suitable for MCP `image` content with `mimeType: image/svg+xml`.

## Building locally

```bash
# Run server directly (requires incplot on PATH)
cd server && make dev

# Build container
podman build -t incplot-server .

# Smoketest against a running server
make smoketest SERVER=http://localhost:8080
```

## Attribution

incplot-server wraps [incplot](https://github.com/InCom-0/incplot) by [InCom-0](https://github.com/InCom-0), a C++ terminal plotting tool licensed under MIT. The container bundles a fork ([mskyttner/incplot-lib](https://github.com/mskyttner/incplot-lib)) that carries a fix for UTF-8 label slicing in horizontal bar charts pending upstream merge.

SVG output uses [ansitosvg](https://github.com/wader/ansisvg) by [wader](https://github.com/wader). Data queries run on [DuckDB](https://duckdb.org/).

## License

MIT — see [LICENSE.txt](LICENSE.txt).
