# Design: gotui & Pure-Go Plot Types for incplot-server

**Date:** 2026-05-05
**Status:** Approved — ready for implementation planning

---

## 1. Overview

incplot-server currently renders all charts via the `incplot` C++ binary, which supports six
types: `line`, `scatter`, `barV`, `barHS`, `barHM`, `barVM`. This design adds two new rendering
layers that cover chart types the binary cannot produce, prioritising compact output that works
in terminals, MCP result boxes, Quarto documents, and plain-text email.

**Rendering layers after this change:**

| Layer | Renderer | Types |
|-------|----------|-------|
| 1 (existing) | incplot binary | `line` `scatter` `barV` `barHS` `barHM` `barVM` |
| 2 (new) | gotui widgets | `heatmap` `treemap` `sparkline` |
| 3 (new) | pure Go | `hist` `box` `barH` |

All nine new types are exposed via the same `type=` parameter on the existing `/incplot/plot`
endpoint and the existing MCP `plot` / `source_plot` tools. No new endpoints or tools.

The subpath remains `/incplot/` for now; a rename to something more descriptive (e.g.
`/textplot/`) is deferred until a suitably concise name is chosen.

---

## 2. Widget Scope & Rationale

**gotui widgets included:**

| Widget | type= | Reason |
|--------|-------|--------|
| Heatmap | `heatmap` | no incplot equivalent; density encoding for numeric matrices |
| TreeMap | `treemap` | no incplot equivalent; area encoding for many-category data |
| SparklineGroup | `sparkline` | no incplot equivalent; compact multi-series time trends |

**gotui widgets excluded from this spec:**
- `BarChart`, `StackedBarChart` — overlap with incplot's existing bar variants; gotui adds
  no meaningful difference for text output
- `PieChart`, `RadarChart` — deferred per Stephen Few: less effective encodings for most
  analytical tasks; not part of the basic compressible chart vocabulary
- `FunnelChart`, `StepChart` — deferred; not priority

**Pure-Go charts included:**

| Chart | type= | Reason |
|-------|-------|--------|
| Histogram | `hist` | distribution shape; fundamental; missing from all current renderers for inline data |
| Box plot | `box` | compact five-number summary; email-safe |
| Horizontal bar | `barH` | explicit-only; email/markdown-native alternative to incplot's `barV` |

---

## 3. Architecture

### 3.1 New and modified files

```
server/
  gotui_render.go     NEW  — renderGotuiPlot, bufToANSI, ansiToAsciinemaHTML,
                             per-widget data mappers (heatmap, treemap, sparkline)
  gotui_patch.go      NEW  — MonochromeMode bool patch for Sparkline widget
  textchart.go        NEW  — renderTextChart, hist/box/barH builders, theme palette
  render.go           MOD  — two new case blocks at top of renderPlot dispatch (~10 lines)
  mcp_infer.go        MOD  — extended inferPlotType with gotui + textchart rules
  mcp.go              MOD  — type enum extended; descriptions updated
  go.mod / go.sum     MOD  — add github.com/metaspartan/gotui/v5 dependency
  vendor/             MOD  — go mod vendor + MonochromeMode patches applied to vendored copy
  assets/
    asciinema-player.min.js   NEW  — embedded at build time via //go:embed
    asciinema-player.min.css  NEW  — embedded at build time via //go:embed
```

### 3.2 Dispatch in `render.go`

```go
func renderPlot(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
    switch opts.PlotType {
    case "hist", "box", "barH":
        renderTextChart(w, src, opts)
        return
    case "heatmap", "treemap", "sparkline":
        renderGotuiPlot(w, src, opts)
        return
    }
    // existing incplot binary path — unchanged
    ...
}
```

The gotui and textchart renderers intercept before the incplot binary is invoked. No existing
code paths are modified below the dispatch.

---

## 4. Data Flow

### 4.1 Common pipeline

```
Client input (CSV or NDJSON)
    │
    ▼
toNDJSON()                         existing — converts CSV if needed
    │
    ▼
inferSchemaFromSample()            existing — []colSchema per column
    │
    ▼
inferPlotType()                    EXTENDED — returns best type string
    │
    ▼
renderPlot() dispatch
    ├── textchart types  → renderTextChart()
    ├── gotui types      → renderGotuiPlot()
    └── incplot types    → existing binary path
```

### 4.2 Auto-inference rules (complete, evaluated top to bottom)

| Condition | Selected type | Renderer |
|-----------|--------------|----------|
| S=0, T=0, N=1, rows≥5 | `hist` | textchart |
| S=0, T=0, N≥2, rows≥10 | `box` | textchart |
| S=0, T=0, N≥3, rows≥3 | `heatmap` | gotui |
| S=1, N=1, rows≥10 | `treemap` | gotui |
| T≥1, N≥4 | `sparkline` | gotui |
| T≥1, N≥1 | `line` | incplot |
| S=1, N=1 | `barV` | incplot |
| S=1, N=2..3 | `barVM` | incplot |
| S=1, N≥4 | `barHS` | incplot |
| S=0, T=0, N=2 | `scatter` | incplot |
| fallback | `line` | incplot |

S = string columns, T = temporal columns, N = numeric columns.
`barH` is explicit-only — not auto-inferred.
Two-column all-numeric data (N=2, S=0, T=0) selects `scatter` rather than `heatmap`
regardless of row count; heatmap requires N≥3 to provide meaningful density contrast.

### 4.3 gotui widget data mapping

Each gotui type maps NDJSON columns to widget data structures:

- **heatmap** — all-numeric columns become the 2D value matrix; column headers become column
  labels; row index becomes row labels
- **treemap** — first string column = node label; first numeric column = node value
- **sparkline** — one `Sparkline` per numeric column; temporal or row-index = x-axis;
  string column (if present) = group label

### 4.4 Terminal dimensions

```
width  = opts.Width  (default 80, range 40–400)
height = round(width × 5/8)   // golden ratio ≈ 0.625; 80→50, 160→100, 40→25
```

This matches the classic 80×50 terminal convention and scales linearly with width.

---

## 5. ANSI Rendering & HTML Output

### 5.1 gotui buffer → ANSI text (`bufToANSI`)

gotui renders into a `ui.Buffer` — a 2D grid of `Cell{Rune, Style}`. Style carries `Fg` and
`Bg` as `ui.Color` (TrueColor RGB or indexed palette).

Conversion walks cells row-by-row, emitting escape sequences only on style changes:

```
for each row:
    for each cell:
        if style changed → emit \x1b[38;2;R;G;Bm (fg) and/or \x1b[48;2;R;G;Bm (bg)
        emit cell.Rune
    emit \n
emit \x1b[0m
```

### 5.2 MonochromeMode and the `raw` parameter

`MonochromeMode` on gotui widgets controls rendering style (not just colour output):

| format | raw | MonochromeMode | Effect |
|--------|-----|----------------|--------|
| text | true (default) | false | space+bg fills → full ANSI 24-bit colour |
| text | false | true | glyph fills (█ ▓ ▒ ░) → visible without colour; ANSI stripped |
| html | — | false | always colour; asciinema player renders ANSI faithfully |

The same `raw` parameter already present on the MCP tools controls `MonochromeMode`
automatically — no new parameter needed.

### 5.3 Asciicast v3 format

Output for `format=html` is a static single-frame asciicast v3 recording:

```
{"version":3,"term":{"cols":80,"rows":50},"title":"incplot"}
[0.0,"o","<json-escaped ANSI string>"]
```

v3 uses relative event intervals (not absolute timestamps); for a single frame both are 0.0.
Generated in Go with no library — `json.Marshal` handles ANSI string escaping correctly.

### 5.4 Self-contained HTML via data URL inlining

The asciicast content is base64-encoded and passed as a data URL to the asciinema player,
following the player's documented server-side generation pattern:

```go
encoded := base64.StdEncoding.EncodeToString([]byte(castNDJSON))
dataURL  := "data:text/plain;base64," + encoded
```

```html
<script>
AsciinemaPlayer.create(
  "data:text/plain;base64,<encoded>",
  document.getElementById("player"),
  {cols: 80, rows: 50, controls: false, autoPlay: true}
);
</script>
```

The player JS + CSS (`asciinema-player.min.js`, `asciinema-player.min.css`) are downloaded
during the container image build and embedded via `//go:embed assets/`. No CDN dependency;
works offline in Quarto documents.

Player is initialised with `controls: false` — renders as a static coloured terminal block
with no playback chrome.

### 5.5 Future SVG path (out of scope)

Since this design produces asciicast v3 natively, adding SVG output in a future iteration
requires only: pipe cast content → `svg-term-cli` stdin → SVG. `svg-term-cli` (Node.js,
github.com/marionebl/svg-term-cli) handles terminal font metrics internally, avoiding the
character-width alignment issues present in the current `ansisvg` path. Requires adding
Node.js to the container image — deferred. The current `ansisvg` SVG path for incplot binary
output is unchanged.

### 5.6 HTML format for pure-Go text charts

Pure-Go charts (`hist`, `box`, `barH`) produce Unicode block characters with no ANSI codes.
Their `format=html` output is a `<pre>` element with monospace font CSS — no asciinema player
needed. This keeps the pages small and dependency-free.

---

## 6. Pure-Go Text Charts (`textchart.go`)

All three charts work in both monochrome and coloured modes. Colour is applied as 24-bit ANSI
foreground codes, harmonised with incplot's `solarized_light` defaults. A palette struct in
`textchart.go` is keyed by theme name, matching the `themeBG` pattern in `main.go`, so other
themes are supported automatically.

**Solarized Light palette used (from incplot's existing ANSI output):**

```go
var solLight = chartPalette{
    Series: [][3]int{
        {133, 153,   0},  // green  — primary series / bar fill
        { 38, 139, 210},  // blue   — secondary
        {220,  50,  47},  // red    — tertiary
    },
    Axis:   [3]int{238, 232, 213},  // base2  — borders, empty fill
    Label:  [3]int{ 88, 110, 117},  // base01 — text labels
}
```

### 6.1 `hist` — Histogram

- Input: one numeric column; rows are observations
- Bins: Sturges' rule `ceil(log2(n))+1`, capped at 20
- Bar width scaled to `opts.Width - 30` characters
- Coloured: bars in solarized blue; axis ticks and counts in base01
- Monochrome: same glyphs, no colour codes

```
[18–27)  ████████████████         45
[27–36)  ████████████████████████ 68
[36–45)  ████████████████████████████████ 91
[45–54)  █████████████████        48
[54–63)  ████████                 22
```

### 6.2 `box` — Box plot

- Input: one or more numeric columns; rows are observations
- Five-number summary: min, Q1, median, Q3, max (sort + linear interpolation)
- One row per column, axis auto-scaled across all columns
- Coloured: box outline in base2, median `|` in green, whiskers in base01;
  multiple columns cycle through series colours
- Monochrome: same characters, no colour codes

```
         min                         max
salary   |----[========|=======]---------|   35k median
age      |--------[======|=====]---------|   42 median
score    |------------[====|=====]-------|   71 median
```

### 6.3 `barH` — Horizontal text bar

- Input: S=1 label column + N=1 numeric column
- Label left-aligned in fixed-width field; bar fills remaining width; value right-aligned
- Explicit-only type — not auto-inferred (incplot's `barV` remains the default for S=1+N=1)
- Coloured: filled `█` in solarized green, empty `░` in base2
- Monochrome: same glyphs, no colour codes

```
Sweden   ████████████████████░░░░  82.3
Norway   ██████████████████░░░░░░  78.1
Denmark  █████████████████░░░░░░░  74.2
Finland  ████████████████░░░░░░░░  71.9
```

---

## 7. Sparkline `MonochromeMode` Patch

The gotui `SparklineGroup` widget receives a `MonochromeMode bool` field following the
identical pattern applied previously to `BarChart`, `StackedBarChart`, `Heatmap`, and
`TreeMap`:

- `false` (default): fill bars using `LineColor` as background on space character
- `true`: fill bars using `▁▂▃▄▅▆▇█` glyphs as foreground on default background

Sparkline already uses `▁▂▃▄▅▆▇█` natively, so the monochrome path is minimal. The patch
lives in `server/gotui_patch.go`.

All five patched widgets are applied to the vendored copy under `server/vendor/` (see §3.1).

---

## 8. Error Handling

gotui `Draw()` does not return errors — it renders silently. Errors arise at data-mapping time.
The response pattern matches the existing incplot error path throughout.

| Failure | Response |
|---------|----------|
| No data rows after parsing | HTTP 400 / mcpErr "no data rows" |
| Column shape incompatible with requested type | HTTP 400 / mcpErr with descriptive message |
| Width < 20 (after opts parsing) | clamp to 20 silently |
| Empty output from buffer or builder | HTTP 500 / mcpErr "renderer produced empty output" |

---

## 9. MCP Schema Updates

`mcp.go` — extended type enum for both `plot` and `source_plot`:

```json
"enum": ["line","scatter","barV","barHS","barHM","barVM",
         "heatmap","treemap","sparkline",
         "hist","box","barH"]
```

Updated description note: *"heatmap/treemap/sparkline are rendered by gotui;
hist/box/barH by a pure-Go renderer; all other types use the incplot binary."*

The existing `raw` parameter (strip ANSI / enable MonochromeMode) applies to all types
unchanged.

---

## 10. Out of Scope

| Item | Reason deferred |
|------|----------------|
| SVG output for gotui/textchart types | svg-term-cli requires Node.js in container; future path once Node.js added |
| `PieChart`, `RadarChart` | Less effective encodings per Few; not part of basic vocabulary |
| `FunnelChart`, `StepChart` | Lower priority; no gap they uniquely fill |
| DuckDB `textplot` integration (`tp_bar`, `tp_sparkline`, `textplot_histogram`) | SQL-native scalar model is complementary; accessible today via `/incplot/data?sql=`; a future `format=textplot` mode could pretty-print results directly |
| Subpath rename (`/incplot/` → `/textplot/` or similar) | Deferred until a suitably concise name is agreed |
| ggsql 0.3.1 (posit-dev) SVG path | Alternative future SVG path; separate spec |
