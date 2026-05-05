# gotui & Pure-Go Plot Types Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add six new `type=` values to `/incplot/plot` and the MCP `plot`/`source_plot` tools — `hist`, `box`, `barH` (pure Go, no new deps) and `heatmap`, `treemap`, `sparkline` (gotui, vendored + patched) — plus two Containerfile improvements: BuildKit C++ build cache and pre-installed DuckDB textplot extension.

**Architecture:** New type renderers intercept at the top of `renderPlot()` before the existing incplot binary dispatch. Pure-Go charts live in `textchart.go`; gotui charts live in `gotui_render.go`. `inferPlotType` gains a second argument (`rows int`) and evaluates new row-count-aware rules before the existing incplot rules. `mcp_plot.go` switches from `renderPlotText` to `renderPlot` so both old and new types route through a single dispatch.

**Tech Stack:** Go 1.25, `github.com/metaspartan/gotui/v5` (vendored + MonochromeMode patches), asciinema player v3.15.1 (embedded via `//go:embed`), asciicast v3 format, BuildKit cache mounts in Containerfile.

---

## File Map

**Create:**
- `server/mcp_infer_test.go` — unit tests for extended `inferPlotType`
- `server/textchart.go` — solarized palette, NDJSON readers, hist/box/barH renderers, `renderTextChart` dispatcher
- `server/textchart_test.go` — unit + rendering tests for textchart helpers
- `server/gotui_patch.go` — documents the SparklineGroup MonochromeMode patch applied in vendor/
- `server/gotui_render.go` — `bufToANSI`, widget data mappers, `renderGotuiPlot`, HTML path via asciinema player
- `server/assets/asciinema-player.min.js` — **not committed**; downloaded by Containerfile + Makefile `assets` target
- `server/assets/asciinema-player.min.css` — same

**Modify:**
- `server/mcp_infer.go:178-204` — `inferPlotType(schema, rows int)` + new rules
- `server/mcp_plot.go:47-65` — pass row count to `inferPlotType`; call `renderPlot` instead of `renderPlotText`
- `server/render.go:156-167` — type dispatch at top of `renderPlot`
- `server/mcp.go:21,35` — extend `"enum"` arrays + updated descriptions
- `server/go.mod`, `server/go.sum` — add `github.com/metaspartan/gotui/v5`
- `server/vendor/github.com/metaspartan/gotui/v5/widgets/sparklinechart.go` (exact filename TBD after `go get`) — add `MonochromeMode bool` field + conditional glyph path
- `Containerfile` — BuildKit C++ build cache; asciinema asset download in go-builder; DuckDB textplot pre-install in runtime stage
- `server/Makefile` — add `assets` target; add `.gitignore` entry for `server/assets/`

---

## Task 1: Extend `inferPlotType` — row-count rules and tests

**Files:**
- Modify: `server/mcp_infer.go:178-204`
- Create: `server/mcp_infer_test.go`

The existing signature `inferPlotType(schema []colSchema) string` (line 178) must gain a second argument `rows int` so the new rules (hist ≥5, box ≥10, heatmap ≥3, treemap ≥10) can fire.

- [ ] **Step 1: Create test file — all cases, will fail to compile**

```go
// server/mcp_infer_test.go
package main

import (
	"fmt"
	"testing"
)

func cols(types ...string) []colSchema {
	out := make([]colSchema, len(types))
	for i, t := range types {
		out[i] = colSchema{Name: fmt.Sprintf("c%d", i), ColType: t}
	}
	return out
}

func TestInferPlotType(t *testing.T) {
	tests := []struct {
		name   string
		schema []colSchema
		rows   int
		want   string
	}{
		// New: hist
		{"hist_basic",     cols("numeric"), 10, "hist"},
		{"hist_min_rows",  cols("numeric"), 5,  "hist"},
		{"hist_too_few",   cols("numeric"), 4,  "line"}, // fallback — not ≥5

		// New: box (multi-numeric, ≥10 rows)
		{"box_two",         cols("numeric", "numeric"), 10, "box"},
		{"box_three",       cols("numeric", "numeric", "numeric"), 10, "box"},
		{"box_too_few",     cols("numeric", "numeric"), 9,  "scatter"}, // N=2 S=0 T=0 → scatter

		// New: heatmap (≥3 numeric, ≥3 rows, no S/T)
		{"heatmap_basic",   cols("numeric", "numeric", "numeric"), 5,  "heatmap"},
		{"heatmap_min",     cols("numeric", "numeric", "numeric"), 3,  "heatmap"},
		{"heatmap_few",     cols("numeric", "numeric", "numeric"), 2,  "line"}, // fallback

		// New: treemap (S=1, N=1, ≥10 rows)
		{"treemap_basic",   []colSchema{{ColType: "string"}, {ColType: "numeric"}}, 10, "treemap"},
		{"treemap_too_few", []colSchema{{ColType: "string"}, {ColType: "numeric"}}, 9,  "barV"},

		// New: sparkline (T≥1, N≥4)
		{"sparkline_basic", cols("temporal", "numeric", "numeric", "numeric", "numeric"), 5, "sparkline"},
		{"sparkline_exact", cols("temporal", "numeric", "numeric", "numeric", "numeric"), 1, "sparkline"},
		{"sparkline_n3",    cols("temporal", "numeric", "numeric", "numeric"), 5, "line"}, // N=3 < 4

		// Existing incplot types (unchanged)
		{"line",  cols("temporal", "numeric"), 5, "line"},
		{"barV",  []colSchema{{ColType: "string"}, {ColType: "numeric"}}, 5, "barV"},
		{"barVM", []colSchema{
			{ColType: "string"}, {ColType: "numeric"}, {ColType: "numeric"},
		}, 5, "barVM"},
		{"barHS", []colSchema{
			{ColType: "string"},
			{ColType: "numeric"}, {ColType: "numeric"},
			{ColType: "numeric"}, {ColType: "numeric"},
		}, 5, "barHS"},
		{"scatter", cols("numeric", "numeric"), 5, "scatter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferPlotType(tt.schema, tt.rows)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run — expect compilation error (wrong arg count)**

```bash
cd server && go test ./... -run TestInferPlotType 2>&1 | head -8
```
Expected: `too many arguments in call to inferPlotType` (compilation failure).

- [ ] **Step 3: Replace `inferPlotType` in `mcp_infer.go` (lines 178–204)**

```go
// inferPlotType maps column-type counts and row count to the best chart type.
// Evaluated top-to-bottom; more specific / data-rich rules come first.
//
//  S=0, T=0, N=1,  rows≥5  → hist      (textchart)
//  S=0, T=0, N≥2,  rows≥10 → box       (textchart)
//  S=0, T=0, N≥3,  rows≥3  → heatmap   (gotui)
//  S=1, N=1,       rows≥10 → treemap   (gotui)
//  T≥1, N≥4               → sparkline (gotui)
//  T≥1, N≥1               → line      (incplot)
//  S=1, N=1               → barV      (incplot)
//  S=1, N=2..3            → barVM     (incplot)
//  S=1, N≥4              → barHS     (incplot)
//  S=0, T=0, N=2          → scatter   (incplot)
//  fallback               → line
func inferPlotType(schema []colSchema, rows int) string {
	var S, T, N int
	for _, c := range schema {
		switch c.ColType {
		case "string":
			S++
		case "temporal":
			T++
		case "numeric":
			N++
		}
	}
	switch {
	case S == 0 && T == 0 && N == 1 && rows >= 5:
		return "hist"
	case S == 0 && T == 0 && N >= 2 && rows >= 10:
		return "box"
	case S == 0 && T == 0 && N >= 3 && rows >= 3:
		return "heatmap"
	case S == 1 && N == 1 && rows >= 10:
		return "treemap"
	case T >= 1 && N >= 4:
		return "sparkline"
	case T >= 1 && N >= 1:
		return "line"
	case S == 1 && N == 1:
		return "barV"
	case S == 1 && N >= 2 && N <= 3:
		return "barVM"
	case S == 1 && N >= 4:
		return "barHS"
	case S == 0 && T == 0 && N == 2:
		return "scatter"
	default:
		return "line"
	}
}
```

- [ ] **Step 4: Fix the call site in `mcp_plot.go`**

In `mcp_plot.go`, around line 47, add row-count computation and pass it:

```go
// Replace:
plotType = inferPlotType(schema)

// With:
rowCount := strings.Count(strings.TrimRight(ndjson, "\n"), "\n") + 1
plotType = inferPlotType(schema, rowCount)
```

- [ ] **Step 5: Run tests — all cases must pass**

```bash
cd server && go test ./... -run TestInferPlotType -v
```
Expected: `--- PASS: TestInferPlotType/hist_basic`, `… PASS` for all 20 cases.

- [ ] **Step 6: Verify full build**

```bash
cd server && go build .
```
Expected: exits 0.

- [ ] **Step 7: Commit**

```bash
git add server/mcp_infer.go server/mcp_infer_test.go server/mcp_plot.go
git commit -m "feat: extend inferPlotType with row-count rules for hist/box/heatmap/treemap/sparkline"
```

---

## Task 2: `textchart.go` — palette, helpers, histogram

**Files:**
- Create: `server/textchart.go`
- Create: `server/textchart_test.go`

- [ ] **Step 1: Write failing tests for pure-math helpers**

```go
// server/textchart_test.go
package main

import (
	"strings"
	"testing"
)

func TestHistBinCount(t *testing.T) {
	tests := []struct{ n, want int }{
		{5, 4},    // ceil(log2(5))+1  = 3+1 = 4
		{10, 5},   // ceil(log2(10))+1 = 4+1 = 5
		{50, 7},   // ceil(log2(50))+1 = 6+1 = 7
		{1000, 11},// ceil(log2(1000))+1 = 10+1 = 11
		{1<<20, 21},// would be 21 but capped at 20
	}
	tests[4].want = 20 // cap at 20
	for _, tt := range tests {
		got := histBinCount(tt.n)
		if got != tt.want {
			t.Errorf("histBinCount(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestFiveNumber(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}
	min, q1, med, q3, max := fiveNumber(data)
	// sorted [1 2 3 4 5]: idx=0→1, idx=1→2, idx=2→3, idx=3→4, idx=4→5
	// quantile(p=0.25) → idx=0.25*(5-1)=1.0 → data[1]=2
	// quantile(p=0.75) → idx=0.75*(5-1)=3.0 → data[3]=4
	if min != 1 { t.Errorf("min: got %v want 1", min) }
	if q1 != 2  { t.Errorf("q1: got %v want 2", q1) }
	if med != 3 { t.Errorf("median: got %v want 3", med) }
	if q3 != 4  { t.Errorf("q3: got %v want 4", q3) }
	if max != 5 { t.Errorf("max: got %v want 5", max) }
}

func TestRenderHistSmoke(t *testing.T) {
	ndjson := strings.Repeat(`{"v":1}`+"\n"+`{"v":2}`+"\n"+`{"v":3}`+"\n"+
		`{"v":4}`+"\n"+`{"v":5}`+"\n", 2) // 10 rows
	schema := []colSchema{{Name: "v", ColType: "numeric"}}
	var sb strings.Builder
	if err := renderHist(&sb, schema, strings.NewReader(ndjson), 80, true); err != nil {
		t.Fatalf("renderHist: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "[") {
		t.Error("expected bracket labels in histogram output")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 4 {
		t.Errorf("expected ≥4 bin rows, got %d: %q", len(lines), out)
	}
}
```

- [ ] **Step 2: Confirm compilation failure**

```bash
cd server && go test ./... -run TestHistBinCount 2>&1 | head -5
```
Expected: undefined `histBinCount`, `fiveNumber`, `renderHist`.

- [ ] **Step 3: Create `server/textchart.go`**

```go
// server/textchart.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// chartPalette holds RGB triples for a theme's chart colours.
type chartPalette struct {
	Series [][3]int // per-series colours, cycled
	Axis   [3]int   // borders / empty fill
	Label  [3]int   // text labels
}

// solLight matches incplot's solarized_light defaults.
var solLight = chartPalette{
	Series: [][3]int{
		{133, 153, 0},  // green  — primary series / bar fill
		{38, 139, 210}, // blue   — secondary
		{220, 50, 47},  // red    — tertiary
	},
	Axis:  [3]int{238, 232, 213}, // base2
	Label: [3]int{88, 110, 117},  // base01
}

func tcFg(r, g, b int) string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b) }
func tcReset() string         { return "\x1b[0m" }

// histBinCount returns the Sturges bin count, capped at 20.
func histBinCount(n int) int {
	k := int(math.Ceil(math.Log2(float64(n)))) + 1
	if k > 20 {
		return 20
	}
	return k
}

// fiveNumber returns min, Q1, median, Q3, max using linear interpolation.
// data is sorted in place.
func fiveNumber(data []float64) (minv, q1, med, q3, maxv float64) {
	sort.Float64s(data)
	n := len(data)
	minv = data[0]
	maxv = data[n-1]
	med = quantile(data, 0.5)
	q1 = quantile(data, 0.25)
	q3 = quantile(data, 0.75)
	return
}

func quantile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}
	idx := p * float64(n-1)
	lo := int(idx)
	if lo+1 >= n {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[lo+1]*frac
}

// readNumericCol reads all values of one numeric column from NDJSON src.
func readNumericCol(src io.Reader, name string) ([]float64, error) {
	raw, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	var vals []float64
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		if v, ok := row[name]; ok {
			switch x := v.(type) {
			case float64:
				vals = append(vals, x)
			case string:
				if f, e := strconv.ParseFloat(x, 64); e == nil {
					vals = append(vals, f)
				}
			}
		}
	}
	return vals, nil
}

// readAllNumericCols returns a slice of (name, values) for every numeric column,
// in schema order. All columns are read from the same NDJSON bytes.
func readAllNumericCols(raw []byte, schema []colSchema) ([]string, [][]float64) {
	var names []string
	for _, c := range schema {
		if c.ColType == "numeric" {
			names = append(names, c.Name)
		}
	}
	cols := make([][]float64, len(names))
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		for i, name := range names {
			if v, ok := row[name]; ok {
				switch x := v.(type) {
				case float64:
					cols[i] = append(cols[i], x)
				case string:
					if f, e := strconv.ParseFloat(x, 64); e == nil {
						cols[i] = append(cols[i], f)
					}
				}
			}
		}
	}
	return names, cols
}

// renderTextChart is the entry point called by renderPlot for hist/box/barH types.
func renderTextChart(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	raw, err := io.ReadAll(src)
	if err != nil {
		http.Error(w, "read source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_, schema, err := toNDJSON(string(raw))
	if err != nil {
		http.Error(w, "parse data: "+err.Error(), http.StatusBadRequest)
		return
	}

	widthInt := 80
	if n, e := strconv.Atoi(opts.Width); e == nil && n >= 20 {
		widthInt = n
	}
	if widthInt < 20 {
		widthInt = 20
	}

	mono := opts.Format != "html" // html uses colour; text mode honours raw param
	// For format=text, colour is the default; mono is set by the caller stripping ANSI.
	// We always emit colour here; mcp_plot.go strips it when raw=false.
	mono = false

	var sb strings.Builder
	switch opts.PlotType {
	case "hist":
		err = renderHist(&sb, schema, strings.NewReader(string(raw)), widthInt, mono)
	case "box":
		err = renderBox(&sb, schema, raw, widthInt, mono)
	case "barH":
		err = renderBarH(&sb, schema, raw, widthInt, mono)
	default:
		http.Error(w, "unknown textchart type: "+opts.PlotType, http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body := sb.String()
	if body == "" {
		http.Error(w, "renderer produced empty output", http.StatusInternalServerError)
		return
	}

	switch opts.Format {
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w,
			`<!DOCTYPE html><html><head><meta charset="utf-8">`+
				`<style>body{background:#fdf6e3;margin:0}pre{font-family:monospace;`+
				`font-size:14px;line-height:1.4;padding:16px;color:#657b83;`+
				`white-space:pre;overflow:auto}</style></head>`+
				`<body><pre>%s</pre></body></html>`, body)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, body)
	}
}

// renderHist writes a histogram of the first numeric column to w.
func renderHist(w io.Writer, schema []colSchema, src io.Reader, width int, mono bool) error {
	colName := ""
	for _, c := range schema {
		if c.ColType == "numeric" {
			colName = c.Name
			break
		}
	}
	if colName == "" {
		return fmt.Errorf("no numeric column for histogram")
	}

	data, err := readNumericCol(src, colName)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("no data rows")
	}

	k := histBinCount(len(data))
	minVal, maxVal := data[0], data[0]
	for _, v := range data[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	if minVal == maxVal {
		maxVal = minVal + 1
	}
	step := (maxVal - minVal) / float64(k)
	counts := make([]int, k)
	for _, v := range data {
		b := int((v - minVal) / step)
		if b >= k {
			b = k - 1
		}
		counts[b]++
	}
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}

	barWidth := width - 30
	if barWidth < 10 {
		barWidth = 10
	}

	blue := solLight.Series[1]
	for i := 0; i < k; i++ {
		lo := minVal + float64(i)*step
		hi := lo + step
		label := fmt.Sprintf("[%-6.4g–%-6.4g)", lo, hi)
		filled := 0
		if maxCount > 0 {
			filled = int(math.Round(float64(counts[i]) * float64(barWidth) / float64(maxCount)))
		}
		bar := strings.Repeat("█", filled)
		if !mono {
			fmt.Fprintf(w, "%s%-18s %s%s  %d\n",
				tcFg(blue[0], blue[1], blue[2]), label, bar, tcReset(), counts[i])
		} else {
			fmt.Fprintf(w, "%-18s %s  %d\n", label, bar, counts[i])
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests — helpers must pass**

```bash
cd server && go test ./... -run "TestHistBinCount|TestFiveNumber|TestRenderHistSmoke" -v
```
Expected: all three PASS.

- [ ] **Step 5: Commit**

```bash
git add server/textchart.go server/textchart_test.go
git commit -m "feat: add textchart.go with palette, hist helpers, and renderHist"
```

---

## Task 3: `textchart.go` — box plot

**Files:**
- Modify: `server/textchart.go` — add `renderBox`
- Modify: `server/textchart_test.go` — add box tests

- [ ] **Step 1: Add failing tests**

Append to `server/textchart_test.go`:

```go
func TestRenderBoxSmoke(t *testing.T) {
	// 15 rows with three numeric columns: a, b, c
	var lines []string
	for i := 1; i <= 15; i++ {
		lines = append(lines, fmt.Sprintf(`{"a":%d,"b":%d,"c":%d}`, i, i*2, i*3))
	}
	ndjson := strings.Join(lines, "\n")
	schema := []colSchema{
		{Name: "a", ColType: "numeric"},
		{Name: "b", ColType: "numeric"},
		{Name: "c", ColType: "numeric"},
	}
	raw := []byte(ndjson)
	var sb strings.Builder
	if err := renderBox(&sb, schema, raw, 80, true); err != nil {
		t.Fatalf("renderBox: %v", err)
	}
	out := sb.String()
	// Each column should produce one output line
	outLines := strings.Split(strings.TrimSpace(out), "\n")
	if len(outLines) != 3 {
		t.Errorf("expected 3 rows (one per column), got %d:\n%s", len(outLines), out)
	}
	// Each line should contain the column name and bracket characters
	for _, name := range []string{"a", "b", "c"} {
		found := false
		for _, l := range outLines {
			if strings.HasPrefix(strings.TrimSpace(l), name) {
				found = true
			}
		}
		if !found {
			t.Errorf("column %q not found in output:\n%s", name, out)
		}
	}
}
```

- [ ] **Step 2: Confirm failure**

```bash
cd server && go test ./... -run TestRenderBoxSmoke 2>&1 | head -5
```
Expected: `undefined: renderBox`.

- [ ] **Step 3: Add `renderBox` to `textchart.go`**

Append after `renderHist`:

```go
// renderBox writes one box-plot row per numeric column.
func renderBox(w io.Writer, schema []colSchema, raw []byte, width int, mono bool) error {
	names, colData := readAllNumericCols(raw, schema)
	if len(names) == 0 {
		return fmt.Errorf("no numeric columns for box plot")
	}

	// Compute global axis span across all columns.
	globalMin, globalMax := math.Inf(1), math.Inf(-1)
	fiveNums := make([][5]float64, len(names))
	for i, data := range colData {
		if len(data) == 0 {
			continue
		}
		mn, q1, med, q3, mx := fiveNumber(append([]float64{}, data...))
		fiveNums[i] = [5]float64{mn, q1, med, q3, mx}
		if mn < globalMin { globalMin = mn }
		if mx > globalMax { globalMax = mx }
	}
	if math.IsInf(globalMin, 1) {
		return fmt.Errorf("no data rows")
	}
	span := globalMax - globalMin
	if span == 0 {
		span = 1
	}

	// Label column: max column name length + 2 spaces.
	labelW := 0
	for _, n := range names {
		if len(n) > labelW { labelW = len(n) }
	}
	labelW += 2

	plotW := width - labelW - 12 // 12 chars reserved for median label
	if plotW < 20 { plotW = 20 }

	scale := func(v float64) int {
		return int(math.Round((v - globalMin) / span * float64(plotW)))
	}

	for i, name := range names {
		fn := fiveNums[i]
		mn, q1, med, q3, mx := fn[0], fn[1], fn[2], fn[3], fn[4]

		pMn  := scale(mn)
		pQ1  := scale(q1)
		pMed := scale(med)
		pQ3  := scale(q3)
		pMx  := scale(mx)

		// Build the box string character by character.
		row := make([]byte, plotW)
		for j := range row { row[j] = ' ' }
		for j := pMn; j <= pMx; j++ {
			if j >= 0 && j < plotW { row[j] = '-' }
		}
		for j := pQ1; j <= pQ3; j++ {
			if j >= 0 && j < plotW { row[j] = '=' }
		}
		if pMn >= 0 && pMn < plotW  { row[pMn] = '|' }
		if pMx >= 0 && pMx < plotW  { row[pMx] = '|' }
		if pMed >= 0 && pMed < plotW { row[pMed] = '|' }

		green := solLight.Series[0]
		label := fmt.Sprintf("%-*s", labelW, name)
		medLabel := fmt.Sprintf("  %.4g med", med)
		if !mono {
			fmt.Fprintf(w, "%s%s%s%s%s\n",
				tcFg(green[0], green[1], green[2]), label,
				string(row), tcReset(), medLabel)
		} else {
			fmt.Fprintf(w, "%s%s%s\n", label, string(row), medLabel)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test**

```bash
cd server && go test ./... -run TestRenderBoxSmoke -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/textchart.go server/textchart_test.go
git commit -m "feat: add renderBox to textchart.go"
```

---

## Task 4: `textchart.go` — horizontal bar chart

**Files:**
- Modify: `server/textchart.go` — add `renderBarH`
- Modify: `server/textchart_test.go` — add barH tests

- [ ] **Step 1: Add failing test**

Append to `server/textchart_test.go`:

```go
func TestRenderBarHSmoke(t *testing.T) {
	ndjson := `{"label":"Sweden","value":82.3}` + "\n" +
		`{"label":"Norway","value":78.1}` + "\n" +
		`{"label":"Denmark","value":74.2}`
	schema := []colSchema{
		{Name: "label", ColType: "string"},
		{Name: "value", ColType: "numeric"},
	}
	var sb strings.Builder
	if err := renderBarH(&sb, schema, []byte(ndjson), 60, true); err != nil {
		t.Fatalf("renderBarH: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "Sweden") {
		t.Error("expected 'Sweden' in output")
	}
	if !strings.Contains(out, "█") {
		t.Error("expected block character in bar output")
	}
}
```

- [ ] **Step 2: Confirm failure**

```bash
cd server && go test ./... -run TestRenderBarHSmoke 2>&1 | head -5
```
Expected: `undefined: renderBarH`.

- [ ] **Step 3: Add `renderBarH` to `textchart.go`**

```go
// renderBarH writes a horizontal text bar chart.
// Requires exactly one string column (labels) and one numeric column (values).
func renderBarH(w io.Writer, schema []colSchema, raw []byte, width int, mono bool) error {
	labelCol, valueCol := "", ""
	for _, c := range schema {
		if c.ColType == "string" && labelCol == "" {
			labelCol = c.Name
		}
		if c.ColType == "numeric" && valueCol == "" {
			valueCol = c.Name
		}
	}
	if labelCol == "" || valueCol == "" {
		return fmt.Errorf("barH requires one string column and one numeric column")
	}

	type row struct {
		label string
		value float64
	}
	var rows []row
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}
		label := fmt.Sprint(obj[labelCol])
		var val float64
		switch x := obj[valueCol].(type) {
		case float64:
			val = x
		case string:
			val, _ = strconv.ParseFloat(x, 64)
		}
		rows = append(rows, row{label, val})
	}
	if len(rows) == 0 {
		return fmt.Errorf("no data rows")
	}

	maxVal := 0.0
	labelW := 0
	for _, r := range rows {
		if r.value > maxVal { maxVal = r.value }
		if len(r.label) > labelW { labelW = len(r.label) }
	}
	if maxVal == 0 { maxVal = 1 }
	if labelW > 20 { labelW = 20 }

	barW := width - labelW - 10
	if barW < 8 { barW = 8 }

	green := solLight.Series[0]
	axis := solLight.Axis
	for _, r := range rows {
		filled := int(math.Round(r.value / maxVal * float64(barW)))
		empty := barW - filled
		label := fmt.Sprintf("%-*s", labelW, r.label)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
		if !mono {
			fmt.Fprintf(w, "%s%s%s  %s%s%s  %s%.4g%s\n",
				tcFg(solLight.Label[0], solLight.Label[1], solLight.Label[2]), label, tcReset(),
				tcFg(green[0], green[1], green[2]), strings.Repeat("█", filled), tcReset(),
				tcFg(axis[0], axis[1], axis[2]), r.value, tcReset())
			_ = bar // colour mode builds the bar inline above
		} else {
			fmt.Fprintf(w, "%-*s  %s  %.4g\n", labelW, r.label, bar, r.value)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test**

```bash
cd server && go test ./... -run TestRenderBarHSmoke -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/textchart.go server/textchart_test.go
git commit -m "feat: add renderBarH to textchart.go"
```

---

## Task 5: Wire textchart into `render.go`; update `mcp_plot.go` and `mcp.go`

**Files:**
- Modify: `server/render.go:156-167`
- Modify: `server/mcp_plot.go:51-57`
- Modify: `server/mcp.go:21,35`

- [ ] **Step 1: Add type dispatch at the top of `renderPlot` in `render.go`**

In `render.go`, replace the `renderPlot` function (lines 156–167) with:

```go
// renderPlot dispatches to the appropriate renderer.
// New chart types intercept BEFORE the format switch so renderTextChart and
// renderGotuiPlot can handle text vs html themselves.
func renderPlot(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	switch opts.PlotType {
	case "hist", "box", "barH":
		renderTextChart(w, src, opts)
		return
	// "heatmap", "treemap", "sparkline" added in a later task.
	}
	switch opts.Format {
	case "svg":
		renderPlotSVG(w, src, opts)
	case "svg2":
		renderPlotSVGFromHTML(w, src, opts)
	case "text":
		renderPlotText(w, src, opts)
	default:
		renderPlotHTML(w, src, opts)
	}
}
```

- [ ] **Step 2: Update `mcp_plot.go` — call `renderPlot` instead of `renderPlotText`**

In `mcpPlotHandler` (around line 51), replace:

```go
rec := httptest.NewRecorder()
renderPlotText(rec, strings.NewReader(ndjson), RenderOptions{
    Format:   "text",
    PlotType: plotType,
    Width:    strconv.Itoa(width),
    Theme:    defaultTheme,
})
```

with:

```go
rec := httptest.NewRecorder()
renderPlot(rec, strings.NewReader(ndjson), RenderOptions{
    Format:   "text",
    PlotType: plotType,
    Width:    strconv.Itoa(width),
    Theme:    defaultTheme,
})
```

- [ ] **Step 3: Extend the `"enum"` arrays and descriptions in `mcp.go`**

In `mcp.go`, replace both occurrences of:

```json
"enum": ["line","scatter","barV","barHS","barHM","barVM"]
```

with:

```json
"enum": ["line","scatter","barV","barHS","barHM","barVM",
         "heatmap","treemap","sparkline",
         "hist","box","barH"]
```

Also update the `Description` field of the `plot` tool to add at the end of the string:

```
 heatmap/treemap/sparkline are rendered by gotui; hist/box/barH by a pure-Go renderer; all other types use the incplot binary. barH is explicit-only — not auto-inferred.
```

Make the same update to `source_plot`'s description.

- [ ] **Step 4: Build**

```bash
cd server && go build .
```
Expected: exits 0.

- [ ] **Step 5: Smoke test — hist via MCP handler path**

```bash
cd server && go test ./... -run "TestInfer|TestHist|TestFiveNumber|TestRenderBox|TestRenderBarH" -v
```
Expected: all tests PASS.

- [ ] **Step 6: Manual integration smoke**

```bash
# Build and run the server locally (needs incplot binary at INCPLOT_BIN path)
cd server && go build . && INCPLOT_BIN=$(which incplot 2>/dev/null || echo /dev/null) ./incplot-server &
SERVER_PID=$!
sleep 1

# Generate 10-row CSV of one numeric column and POST via curl
python3 -c "
import random, sys
print('value')
for i in range(10): print(random.gauss(50,10))
" | \
curl -s --data-binary @- \
  "http://localhost:8080/incplot/plot?type=hist&format=text&width=80" \
  -H "Content-Type: text/plain" -X POST 2>/dev/null || \
echo "(POST not supported — use source= param in normal use)"

kill $SERVER_PID 2>/dev/null
```

Note: the `/incplot/plot` endpoint uses `source=URL` not POST body; the integration test above is illustrative. The correct path is via MCP or `source=` pointing to a `/incplot/data?sql=` or `/incplot/source/` URL. Verify via the MCP tool once the server is running in the container.

- [ ] **Step 7: Commit**

```bash
git add server/render.go server/mcp_plot.go server/mcp.go
git commit -m "feat: wire textchart types into render.go dispatch; extend mcp.go enum"
```

---

## Task 6: Vendor `gotui` and verify it builds

**Files:**
- Modify: `server/go.mod`, `server/go.sum`
- Create: `server/vendor/` (go mod vendor output)

- [ ] **Step 1: Fetch gotui**

```bash
cd server && go get github.com/metaspartan/gotui/v5
```
Expected: `go: added github.com/metaspartan/gotui/v5 vX.Y.Z`.

- [ ] **Step 2: Vendor dependencies**

```bash
cd server && go mod vendor
```
Expected: `server/vendor/github.com/metaspartan/gotui/` directory created.

- [ ] **Step 3: Identify the SparklineGroup widget file**

```bash
find server/vendor/github.com/metaspartan/gotui -name "*.go" | xargs grep -l "SparklineGroup\|Sparkline" 2>/dev/null
```
Expected: something like `server/vendor/github.com/metaspartan/gotui/v5/widgets/sparklinechart.go`.

Note the exact path — it is needed in Task 7. Also note the struct field names and Draw() signature used by Heatmap, TreeMap, BarChart so you can mirror the MonochromeMode pattern.

- [ ] **Step 4: Read the MonochromeMode pattern from an already-patched widget**

```bash
grep -n "MonochromeMode" server/vendor/github.com/metaspartan/gotui/v5/widgets/*.go | head -20
```
Expected: lines like `MonochromeMode bool` field declarations and `if w.MonochromeMode {` render branches in Heatmap, TreeMap, BarChart, StackedBarChart.

- [ ] **Step 5: Verify clean build with vendored gotui**

```bash
cd server && go build -mod=vendor .
```
Expected: exits 0 (even though gotui is imported only by `_ "github.com/metaspartan/gotui/v5"` placeholder — add that import to the gotui_render.go stub below if needed).

- [ ] **Step 6: Create minimal `gotui_render.go` stub to confirm import compiles**

```go
// server/gotui_render.go
package main

// Stub — full implementation follows in Tasks 8–10.
// Import here to confirm vendored gotui resolves correctly.
import (
	_ "github.com/metaspartan/gotui/v5"
)
```

Build again:

```bash
cd server && go build -mod=vendor .
```

If gotui requires a specific initialization import path, adjust the import string to match what `go get` printed.

- [ ] **Step 7: Commit**

```bash
git add server/go.mod server/go.sum server/vendor/ server/gotui_render.go
git commit -m "feat: vendor github.com/metaspartan/gotui/v5"
```

---

## Task 7: Patch `SparklineGroup` — add `MonochromeMode`

**Files:**
- Modify: `server/vendor/github.com/metaspartan/gotui/v5/widgets/<sparkline-file>.go`
- Create: `server/gotui_patch.go`

This task modifies the vendored widget source to add the same `MonochromeMode bool` field + conditional rendering that Heatmap/TreeMap/BarChart/StackedBarChart already have. Read those existing patches first (Task 6 Step 4) and apply the identical pattern.

- [ ] **Step 1: Open the SparklineGroup file and find the struct + Draw function**

```bash
# Confirm file path from Task 6 Step 3
SPARK_FILE=$(find server/vendor/github.com/metaspartan/gotui -name "*.go" | xargs grep -l "SparklineGroup" 2>/dev/null | head -1)
echo $SPARK_FILE
grep -n "type SparklineGroup\|MonochromeMode\|func.*Draw\|▁▂▃▄▅▆▇█" $SPARK_FILE | head -20
```

- [ ] **Step 2: Add `MonochromeMode bool` field to the `SparklineGroup` struct**

In `$SPARK_FILE`, locate the struct definition, e.g.:

```go
type SparklineGroup struct {
    Block
    Sparklines []*Sparkline
    // ... other fields
}
```

Add the field:

```go
type SparklineGroup struct {
    Block
    Sparklines     []*Sparkline
    MonochromeMode bool
    // ... other fields
}
```

- [ ] **Step 3: Add `MonochromeMode bool` field to the `Sparkline` struct as well (if it's separate)**

```bash
grep -n "type Sparkline " $SPARK_FILE
```

If `Sparkline` is a separate struct that controls per-series rendering, add `MonochromeMode bool` there too. Mirror exactly what was done for `BarChart` / `StackedBarChart`.

- [ ] **Step 4: Update the Draw logic for monochrome glyph rendering**

Sparklines already use `▁▂▃▄▅▆▇█` glyphs in their normal render path. For MonochromeMode=true the change is minimal: use the glyph as foreground on default background instead of as background-coloured space. Find the cell-fill loop and add the conditional. Mirror the pattern from BarChart:

```go
// Existing (colour mode): set glyph as space with background colour
// cell.Rune = ' '; cell.Style.Bg = lineColor

// Add MonochromeMode branch:
if sg.MonochromeMode {
    // glyph fill: visible without colour
    cell.Rune = glyphForHeight(...)  // use existing glyph logic
    cell.Style.Fg = lineColor
    cell.Style.Bg = ColorClear
} else {
    cell.Rune = ' '
    cell.Style.Bg = lineColor
}
```

The exact variable names must match what's in the file. Read the existing glyph selection code carefully.

- [ ] **Step 5: Build with the patch applied**

```bash
cd server && go build -mod=vendor .
```
Expected: exits 0.

- [ ] **Step 6: Create `server/gotui_patch.go`** (documentation + type alias)

```go
// server/gotui_patch.go
//
// MonochromeMode patches applied to vendored gotui widgets:
//
//   vendor/…/widgets/barchart.go        — MonochromeMode bool (existing patch)
//   vendor/…/widgets/stackedbarchart.go — MonochromeMode bool (existing patch)
//   vendor/…/widgets/heatmap.go         — MonochromeMode bool (existing patch)
//   vendor/…/widgets/treemap.go         — MonochromeMode bool (existing patch)
//   vendor/…/widgets/sparklinechart.go  — MonochromeMode bool (this task)
//
// When MonochromeMode=true each widget uses glyph fills (█ ▓ ▒ ░ / ▁▂▃▄▅▆▇█)
// on the default background instead of coloured space characters. This makes
// the output readable without ANSI colour support (email, plain-text terminals).
//
// Re-apply after any `go mod vendor` re-run.
package main
```

- [ ] **Step 7: Commit**

```bash
git add server/vendor/ server/gotui_patch.go
git commit -m "feat: apply SparklineGroup MonochromeMode patch to vendored gotui"
```

---

## Task 8: `gotui_render.go` — `bufToANSI`

**Files:**
- Modify: `server/gotui_render.go`

`bufToANSI` converts a gotui `ui.Buffer` (2D grid of `Cell{Rune, Style}`) to an ANSI escape string. It walks cells row-by-row, emitting escape codes only on style changes.

Before writing code, run this to see the exact Buffer / Cell / Style / Color types:

```bash
grep -n "type Buffer\|type Cell\|type Style\|type Color\|\.Rune\|\.Fg\|\.Bg\|ColorClear" \
  server/vendor/github.com/metaspartan/gotui/v5/*.go \
  server/vendor/github.com/metaspartan/gotui/v5/buffer/*.go 2>/dev/null | head -30
```

Use the exact field and type names from that output in the code below.

- [ ] **Step 1: Replace `gotui_render.go` stub with real implementation (bufToANSI only)**

```go
// server/gotui_render.go
package main

import (
	"fmt"
	"net/http"
	"io"
	"strings"

	ui "github.com/metaspartan/gotui/v5"
)

// bufToANSI converts a rendered gotui Buffer to an ANSI-escaped string.
// It emits \x1b[38;2;R;G;Bm (fg) and \x1b[48;2;R;G;Bm (bg) only on style
// changes, and resets at end of output.
func bufToANSI(buf *ui.Buffer) string {
	rect := buf.GetRect()
	var sb strings.Builder

	// Track last-emitted style to suppress redundant escape codes.
	type emittedStyle struct{ fg, bg ui.Color }
	last := emittedStyle{fg: ui.ColorClear, bg: ui.ColorClear}

	emitColor := func(code int, c ui.Color) {
		// gotui Color is an int32; positive values encode 24-bit RGB:
		// bits [23:16]=R [15:8]=G [7:0]=B, bit 24 set means "TrueColor".
		// Indexed palette colours (0–255) use a different encoding.
		// Check gotui source for exact bit layout and adjust if needed.
		if c <= 0 {
			return // ColorClear / default — don't emit
		}
		r := (uint32(c) >> 16) & 0xff
		g := (uint32(c) >> 8) & 0xff
		b := uint32(c) & 0xff
		fmt.Fprintf(&sb, "\x1b[%d;2;%d;%d;%dm", code, r, g, b)
	}

	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			cell := buf.GetCell(image.Pt(x, y))
			fg := cell.Style.Fg
			bg := cell.Style.Bg
			if fg != last.fg || bg != last.bg {
				if fg != last.fg { emitColor(38, fg) }
				if bg != last.bg { emitColor(48, bg) }
				last = emittedStyle{fg, bg}
			}
			sb.WriteRune(cell.Rune)
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("\x1b[0m")
	return sb.String()
}

// renderGotuiPlot is the entry point called by renderPlot for heatmap/treemap/sparkline.
// Stub — full implementation follows in Tasks 9–10.
func renderGotuiPlot(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	http.Error(w, "gotui renderer not yet implemented", http.StatusNotImplemented)
}
```

**Important:** After writing this, check the exact `ui.Buffer` API:

```bash
grep -n "func.*GetCell\|func.*GetRect\|func.*Buffer\|type Buffer" \
  server/vendor/github.com/metaspartan/gotui/v5/*.go \
  server/vendor/github.com/metaspartan/gotui/v5/buffer/*.go 2>/dev/null
```

Adjust method names, import paths, and the `image.Pt` coordinate type to match what gotui actually provides. If `Buffer` uses a flat cell slice indexed by `(y*width + x)`, adjust accordingly. Add `"image"` to the imports if `image.Pt` is used.

- [ ] **Step 2: Build**

```bash
cd server && go build -mod=vendor .
```
Expected: exits 0. Fix any type mismatches discovered in Step 1.

- [ ] **Step 3: Commit**

```bash
git add server/gotui_render.go
git commit -m "feat: add bufToANSI to gotui_render.go"
```

---

## Task 9: `gotui_render.go` — widget data mappers

**Files:**
- Modify: `server/gotui_render.go`

Before writing code, read the actual widget constructors and field types:

```bash
grep -n "func New\|type Heatmap\|type TreeMap\|type SparklineGroup\|type Sparkline\b" \
  server/vendor/github.com/metaspartan/gotui/v5/widgets/*.go | head -40
```

Also read the data field types:

```bash
grep -n "\.Data\b\|XLabels\|YLabels\|Title\b\|LineColor" \
  server/vendor/github.com/metaspartan/gotui/v5/widgets/*.go | head -40
```

Use those exact field names in the mappers below; adjust where the actual API differs.

- [ ] **Step 1: Add width/height helper and widget data mappers to `gotui_render.go`**

Append to `gotui_render.go` (after `bufToANSI`):

```go
import (
	// add to existing imports:
	"math"
	"strconv"
	"encoding/json"
)

func gotuiDims(opts RenderOptions) (w, h int) {
	w = 80
	if n, err := strconv.Atoi(opts.Width); err == nil && n >= 20 {
		w = n
	}
	if w < 20 { w = 20 }
	h = int(math.Round(float64(w) * 5.0 / 8.0))
	if h < 10 { h = 10 }
	return
}

// heatmapWidget builds a gotui Heatmap from NDJSON bytes and schema.
// All numeric columns form the 2D value matrix; column names become X labels;
// row indices become Y labels.
func heatmapWidget(raw []byte, schema []colSchema, mono bool) (*ui.HeatMap, error) {
	names, colData := readAllNumericCols(raw, schema)
	if len(names) == 0 {
		return nil, fmt.Errorf("no numeric columns for heatmap")
	}

	rows := len(colData[0])
	if rows == 0 {
		return nil, fmt.Errorf("no data rows")
	}

	// Build row-major matrix: data[row][col]
	matrix := make([][]float64, rows)
	for r := 0; r < rows; r++ {
		matrix[r] = make([]float64, len(names))
		for c, col := range colData {
			if r < len(col) {
				matrix[r][c] = col[r]
			}
		}
	}

	hm := ui.NewHeatMap()
	hm.Title = "Heatmap"
	hm.Data = matrix        // verify field name against vendored source
	hm.XLabels = names      // verify
	yLabels := make([]string, rows)
	for i := range yLabels { yLabels[i] = strconv.Itoa(i + 1) }
	hm.YLabels = yLabels    // verify
	hm.MonochromeMode = mono
	return hm, nil
}

// treemapWidget builds a gotui TreeMap from NDJSON bytes and schema.
// First string column = labels; first numeric column = values.
func treemapWidget(raw []byte, schema []colSchema, mono bool) (*ui.TreeMap, error) {
	labelCol, valueCol := "", ""
	for _, c := range schema {
		if c.ColType == "string" && labelCol == "" { labelCol = c.Name }
		if c.ColType == "numeric" && valueCol == "" { valueCol = c.Name }
	}
	if labelCol == "" || valueCol == "" {
		return nil, fmt.Errorf("treemap requires one string and one numeric column")
	}

	data := map[string]float64{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" { continue }
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil { continue }
		label := fmt.Sprint(row[labelCol])
		switch x := row[valueCol].(type) {
		case float64:
			data[label] = x
		case string:
			if f, e := strconv.ParseFloat(x, 64); e == nil { data[label] = f }
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no data rows")
	}

	tm := ui.NewTreeMap()
	tm.Title = "TreeMap"
	tm.Data = data           // verify field name
	tm.MonochromeMode = mono
	return tm, nil
}

// sparklineWidget builds a gotui SparklineGroup from NDJSON bytes and schema.
// One Sparkline per numeric column. Temporal or row-index = X axis.
func sparklineWidget(raw []byte, schema []colSchema, mono bool) (*ui.SparklineGroup, error) {
	names, colData := readAllNumericCols(raw, schema)
	if len(names) == 0 {
		return nil, fmt.Errorf("no numeric columns for sparkline")
	}

	colors := []ui.Color{
		ui.Color(0x859900), // solarized green
		ui.Color(0x268bd2), // solarized blue
		ui.Color(0xdc322f), // solarized red
	}

	var sparklines []*ui.Sparkline
	for i, name := range names {
		sp := ui.NewSparkline()
		sp.Title = name
		sp.Data = colData[i]
		sp.LineColor = colors[i%len(colors)]
		sp.MonochromeMode = mono // if Sparkline struct has this field
		sparklines = append(sparklines, sp)
	}

	sg := ui.NewSparklineGroup(sparklines...)
	sg.Title = "Sparklines"
	sg.MonochromeMode = mono
	return sg, nil
}
```

**API verification note:** After pasting this code, run:

```bash
cd server && go build -mod=vendor . 2>&1
```

Fix all type errors by looking at the actual vendored widget constructors and field names. Common issues:
- `ui.NewHeatMap()` vs `ui.NewHeatmap()` (capitalisation)
- `hm.Data` field type might be `[][]float64` or a named type
- `ui.TreeMap` might use `ui.NewTreeMap()` or a builder pattern
- `ui.Color` encoding may differ from the assumed 24-bit RGB above — check how `ui.ColorRed` etc. are defined
- `ui.NewSparklineGroup(sparklines...)` vs `sg.Sparklines = sparklines`

- [ ] **Step 2: Build — fix all type errors**

```bash
cd server && go build -mod=vendor . 2>&1
```

Iterate until exits 0.

- [ ] **Step 3: Commit**

```bash
git add server/gotui_render.go
git commit -m "feat: add gotui widget data mappers (heatmap/treemap/sparkline)"
```

---

## Task 10: `gotui_render.go` — `renderGotuiPlot` (text output)

**Files:**
- Modify: `server/gotui_render.go`

Before implementing, read how gotui's rendering pipeline works:

```bash
grep -n "func.*Init\|func.*Render\|func.*Draw\|ui\.Init\|ui\.Render\|ui\.NewBuffer\|NewBuffer\|SetRect" \
  server/vendor/github.com/metaspartan/gotui/v5/*.go | head -30
```

gotui was designed as a TUI library that manages the real terminal. For headless rendering we need to bypass the terminal initialisation and render directly to a buffer. The typical approach is `ui.NewBuffer(image.Rect(0, 0, width, height))` + `widget.SetRect(...)` + `widget.Draw(buf)`. Confirm the exact calls from the vendored source.

- [ ] **Step 1: Replace the `renderGotuiPlot` stub in `gotui_render.go`**

```go
func renderGotuiPlot(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	raw, err := io.ReadAll(src)
	if err != nil {
		http.Error(w, "read source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_, schema, err := toNDJSON(string(raw))
	if err != nil {
		http.Error(w, "parse data: "+err.Error(), http.StatusBadRequest)
		return
	}

	width, height := gotuiDims(opts)
	// mono=true when raw=false (no ANSI) — controlled by mcp_plot.go stripping.
	// Here we always render in colour; stripping is the caller's job for text output.
	// For HTML we never strip, so always colour.
	mono := false

	buf := ui.NewBuffer(image.Rect(0, 0, width, height))

	var renderErr error
	switch opts.PlotType {
	case "heatmap":
		var hm *ui.HeatMap
		hm, renderErr = heatmapWidget(raw, schema, mono)
		if renderErr == nil {
			hm.SetRect(0, 0, width, height)
			hm.Draw(buf)
		}
	case "treemap":
		var tm *ui.TreeMap
		tm, renderErr = treemapWidget(raw, schema, mono)
		if renderErr == nil {
			tm.SetRect(0, 0, width, height)
			tm.Draw(buf)
		}
	case "sparkline":
		var sg *ui.SparklineGroup
		sg, renderErr = sparklineWidget(raw, schema, mono)
		if renderErr == nil {
			sg.SetRect(0, 0, width, height)
			sg.Draw(buf)
		}
	default:
		http.Error(w, "unknown gotui type: "+opts.PlotType, http.StatusBadRequest)
		return
	}

	if renderErr != nil {
		http.Error(w, renderErr.Error(), http.StatusBadRequest)
		return
	}

	ansi := bufToANSI(buf)
	if strings.TrimSpace(ansi) == "" {
		http.Error(w, "renderer produced empty output", http.StatusInternalServerError)
		return
	}

	switch opts.Format {
	case "html":
		// HTML path implemented in Task 11.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<pre>%s</pre>", ansi) // placeholder until Task 11
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, ansi)
	}
}
```

**Buffer API note:** `ui.NewBuffer(image.Rect(...))` is the assumed API. Verify:

```bash
grep -n "func NewBuffer\|NewBuffer" server/vendor/github.com/metaspartan/gotui/v5/*.go \
  server/vendor/github.com/metaspartan/gotui/v5/buffer/*.go 2>/dev/null
```

Adjust the constructor call, `SetRect`, and `Draw` signatures as needed.

- [ ] **Step 2: Wire gotui types into `render.go` dispatch**

In `render.go`, update the `renderPlot` function to include the gotui case:

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
	switch opts.Format {
	// ... existing format dispatch unchanged
	}
}
```

- [ ] **Step 3: Build**

```bash
cd server && go build -mod=vendor .
```
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add server/gotui_render.go server/render.go
git commit -m "feat: implement renderGotuiPlot with text output; wire gotui types into dispatch"
```

---

## Task 11: Asciinema player assets and HTML output

**Files:**
- Create: `server/assets/asciinema-player.min.js` (downloaded, not committed)
- Create: `server/assets/asciinema-player.min.css` (downloaded, not committed)
- Modify: `server/gotui_render.go` — replace placeholder HTML with asciinema player output
- Modify: `server/Makefile` — add `assets` target
- Create: `server/assets/.gitignore`

The asciinema player renders asciicast v3 NDJSON. The cast is base64-encoded as a data URL so no server is needed at view time — fully self-contained HTML.

- [ ] **Step 1: Add `assets` Makefile target**

Append to `server/Makefile`:

```makefile
ASCIINEMA_VERSION := 3.15.1
ASCIINEMA_BASE    := https://github.com/asciinema/asciinema-player/releases/download/v$(ASCIINEMA_VERSION)

assets: assets/asciinema-player.min.js assets/asciinema-player.min.css

assets/asciinema-player.min.js:
	mkdir -p assets
	curl -fsSLo $@ $(ASCIINEMA_BASE)/asciinema-player.min.js

assets/asciinema-player.min.css:
	mkdir -p assets
	curl -fsSLo $@ $(ASCIINEMA_BASE)/asciinema-player.min.css
```

- [ ] **Step 2: Create `server/assets/.gitignore`**

```
# Downloaded at build time by Containerfile and `make assets`.
# Not committed — re-fetch with: make assets
asciinema-player.min.js
asciinema-player.min.css
```

- [ ] **Step 3: Download assets locally**

```bash
cd server && make assets
```
Expected: `server/assets/asciinema-player.min.js` and `.min.css` created.

- [ ] **Step 4: Add `//go:embed` declaration and `ansiToAsciinemaHTML` to `gotui_render.go`**

Add at the top of `gotui_render.go` (before `package main` is fine, but `//go:embed` must be in the same package):

```go
package main

import (
	// add to existing imports:
	"encoding/base64"
	_ "embed"
)

//go:embed assets/asciinema-player.min.js
var asciinemaPlayerJS []byte

//go:embed assets/asciinema-player.min.css
var asciinemaPlayerCSS []byte
```

Then add the HTML builder:

```go
// ansiToAsciinemaHTML wraps an ANSI string in a self-contained HTML page
// using the embedded asciinema player and asciicast v3 data URL inlining.
func ansiToAsciinemaHTML(ansi string, cols, rows int, fragment bool) string {
	// Escape the ANSI string for JSON embedding.
	castEvent, _ := json.Marshal(ansi)
	cast := fmt.Sprintf(
		`{"version":3,"term":{"cols":%d,"rows":%d},"title":"incplot"}`+"\n"+
			`[0.0,"o",%s]`+"\n",
		cols, rows, string(castEvent),
	)
	encoded := base64.StdEncoding.EncodeToString([]byte(cast))
	dataURL := "data:text/plain;base64," + encoded

	playerDiv := fmt.Sprintf(
		`<div id="player" style="width:100%%"></div>`+
			`<script>%s</script>`+
			`<link rel="stylesheet" href="data:text/css;base64,%s">`+
			`<script>`+
			`AsciinemaPlayer.create(%q,document.getElementById("player"),`+
			`{cols:%d,rows:%d,controls:false,autoPlay:true,loop:false});`+
			`</script>`,
		string(asciinemaPlayerJS),
		base64.StdEncoding.EncodeToString(asciinemaPlayerCSS),
		dataURL, cols, rows,
	)

	if fragment {
		return playerDiv
	}
	return fmt.Sprintf(
		`<!DOCTYPE html><html><head><meta charset="utf-8">`+
			`<style>html,body{margin:0;padding:0;background:#fdf6e3}</style>`+
			`</head><body>%s</body></html>`,
		playerDiv,
	)
}
```

- [ ] **Step 5: Replace the placeholder HTML branch in `renderGotuiPlot`**

In `renderGotuiPlot`, replace:

```go
case "html":
    // HTML path implemented in Task 11.
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprintf(w, "<pre>%s</pre>", ansi) // placeholder until Task 11
```

with:

```go
case "html":
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    html := ansiToAsciinemaHTML(ansi, width, height, opts.Fragment)
    fmt.Fprint(w, html)
```

- [ ] **Step 6: Update `renderTextChart` HTML branch to use proper styling**

In `textchart.go`, the existing HTML branch already produces a `<pre>` page. Verify it handles `opts.Fragment` correctly. If it doesn't, add:

```go
case "html":
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    css := `<style>body{background:#fdf6e3;margin:0}` +
        `pre{font-family:"Adwaita Mono","JetBrains Mono",monospace;` +
        `font-size:14px;line-height:1.4;padding:16px;white-space:pre}</style>`
    if opts.Fragment {
        fmt.Fprintf(w, "%s<pre>%s</pre>", css, body)
    } else {
        fmt.Fprintf(w,
            `<!DOCTYPE html><html><head><meta charset="utf-8">%s</head>`+
                `<body><pre>%s</pre></body></html>`, css, body)
    }
```

- [ ] **Step 7: Build**

```bash
cd server && go build -mod=vendor .
```
Expected: exits 0. If `//go:embed` fails because assets are missing, run `make assets` first.

- [ ] **Step 8: Commit**

```bash
git add server/gotui_render.go server/textchart.go server/Makefile server/assets/.gitignore
git commit -m "feat: add asciinema v3 HTML output for gotui charts; embed player assets"
```

---

## Task 12: Containerfile — BuildKit cache, asciinema assets, DuckDB textplot

**Files:**
- Modify: `Containerfile`

Three independent changes to the Containerfile:

1. **BuildKit cache for C++ build** — adds `--mount=type=cache` to the CMake build `RUN` step so Ninja's incremental rebuild reuses cached object files across image rebuilds. Requires `# syntax=docker/dockerfile:1` directive at the top (enables BuildKit features).

2. **Asciinema player assets** — downloaded in the go-builder stage before `go build` so `//go:embed` succeeds.

3. **DuckDB textplot extension** — pre-installed in the runtime stage so the first call doesn't need internet access.

- [ ] **Step 1: Add BuildKit syntax directive and cache mounts**

At the very top of `Containerfile`, add:

```dockerfile
# syntax=docker/dockerfile:1
```

In **Stage 2 (builder / C++ incplot)**, replace the CMake build `RUN` step:

```dockerfile
# BEFORE:
RUN cmake -G Ninja \
      -DCMAKE_BUILD_TYPE=Release \
      -DCMAKE_C_COMPILER=gcc-14 \
      -DCMAKE_CXX_COMPILER=g++-14 \
      -B build \
    && cmake --build build -j$(nproc) \
    && cmake --install build --prefix /usr/local

# AFTER:
RUN --mount=type=cache,target=/src/build \
    cmake -G Ninja \
      -DCMAKE_BUILD_TYPE=Release \
      -DCMAKE_C_COMPILER=gcc-14 \
      -DCMAKE_CXX_COMPILER=g++-14 \
      -B build \
    && cmake --build build -j$(nproc) \
    && cmake --install build --prefix /usr/local
```

The cache at `/src/build` persists the CMake cache, Ninja build graph, and compiled `.o` files across container rebuilds. When only Go or runtime files change, the C++ stage uses the cached artifacts and rebuilds in seconds.

Also add a Go module download cache in **Stage 1 (go-builder)**:

```dockerfile
# BEFORE:
RUN CGO_ENABLED=0 go build -o incplot-server .

# AFTER:
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -mod=vendor -o incplot-server .
```

- [ ] **Step 2: Add asciinema asset download to go-builder stage**

In Stage 1, after `COPY server/ .` and before the `RUN go build` step, add:

```dockerfile
ARG ASCIINEMA_VERSION=3.15.1
RUN --mount=type=cache,target=/root/.cache/curl \
    mkdir -p assets \
    && curl -fsSLo assets/asciinema-player.min.js \
       https://github.com/asciinema/asciinema-player/releases/download/v${ASCIINEMA_VERSION}/asciinema-player.min.js \
    && curl -fsSLo assets/asciinema-player.min.css \
       https://github.com/asciinema/asciinema-player/releases/download/v${ASCIINEMA_VERSION}/asciinema-player.min.css
```

The go-builder base image (`golang:1.26.1-bookworm`) already has `curl`. Verify:

```bash
docker run --rm golang:1.26.1-bookworm which curl
```

If absent, add `apt-get install -y --no-install-recommends curl` before the download step.

- [ ] **Step 3: Pre-install DuckDB textplot extension in the runtime stage**

In Stage 3 (runtime), after the existing font installation and before the `COPY` steps, add:

```dockerfile
# Pre-install DuckDB community extensions so first use is offline.
RUN duckdb -c "INSTALL textplot FROM community; LOAD textplot; SELECT 'textplot ok';"
```

This runs DuckDB CLI, downloads the textplot extension binary, stores it in DuckDB's extension cache (`/root/.duckdb/extensions/`), and verifies it loads cleanly. The pre-installed extension is available to all subsequent DuckDB calls in the container.

Note: this step requires internet access during the container build. If the build environment is airgapped, omit this step and install the extension at runtime via DuckDB's `INSTALL` command instead.

- [ ] **Step 4: Build and verify the container**

```bash
# BuildKit must be enabled; podman enables it via DOCKER_BUILDKIT or by default.
podman build --progress=plain -t incplot-server:local -f Containerfile . 2>&1 | tail -20
```
Expected: build succeeds; observe the C++ stage completing in ≤5 min on first build.

Rebuild immediately to verify cache works:

```bash
podman build --progress=plain -t incplot-server:local -f Containerfile . 2>&1 | grep "CACHED\|RUN"
```
Expected: the CMake build step shows `CACHED` and completes in seconds.

- [ ] **Step 5: Smoke test the container**

```bash
podman run -d --rm --name incplot-server -p 8080:8080 incplot-server:local
sleep 2
# hist via HTTP
python3 -c "
print('value')
import random
for _ in range(20): print(round(random.gauss(50, 10), 2))
" | curl -s --data-binary @- \
  "http://localhost:8080/incplot/plot?type=hist&format=text&width=80"
podman stop incplot-server
```

Note: the server uses `source=URL` not POST; test via MCP or a named source URL in practice.

- [ ] **Step 6: Commit**

```bash
git add Containerfile
git commit -m "build: add BuildKit cache mounts for C++ and Go builds; embed asciinema assets; pre-install DuckDB textplot"
```

---

## Self-Review

After writing this plan, checking against the spec (`docs/superpowers/specs/2026-05-05-gotui-plot-types.md`):

**Spec coverage:**
- §2 Widget scope — all six new types covered (hist/box/barH in Tasks 2–5; heatmap/treemap/sparkline in Tasks 6–10)
- §3 Architecture — `gotui_render.go`, `textchart.go`, `gotui_patch.go` created; `render.go` dispatch in Task 5 + Task 10; `mcp_infer.go` in Task 1; `mcp.go` in Task 5
- §4 Data flow — `inferPlotType` extended in Task 1; MCP plot handler updated in Tasks 1 + 5; dispatch chain in Task 10
- §4.3 gotui data mapping — heatmap/treemap/sparkline mappers in Task 9
- §4.4 Terminal dimensions — `gotuiDims()` in Task 10 implements `round(width × 5/8)`
- §5 ANSI rendering — `bufToANSI` in Task 8; MonochromeMode via existing `stripANSI` in `mcp_plot.go` (no change needed); HTML path in Task 11
- §5.3 Asciicast v3 — `ansiToAsciinemaHTML` in Task 11
- §5.4 Data URL inlining — Task 11
- §5.6 HTML for textchart — Task 11 Step 6
- §6 Pure-Go charts — Tasks 2–4
- §7 Sparkline patch — Task 7
- §8 Error handling — 400 on bad column shape, 500 on empty output in `renderTextChart` and `renderGotuiPlot`
- §9 MCP schema — Task 5 Step 3

**Additional requests (not in spec):**
- BuildKit C++ cache — Task 12 Step 1
- Go module cache — Task 12 Step 1
- DuckDB textplot preinstall — Task 12 Step 3

**Gaps found:**
- `barH` not yet added to `renderTextChart`'s HTML fragment handling — covered in Task 11 Step 6 (applies to all textchart types).
- The `readNumericCol` function in `textchart.go` resets `src` state; Task 9 data mappers use `readAllNumericCols` from bytes (not an `io.Reader`), which is consistent.
- `gotui_render.go` imports `image` for `image.Rect` / `image.Pt` — this is in the Go standard library and doesn't need vendoring. Add `"image"` to the import block.
- `renderBarH` in `textchart.go` (Task 4) renders the coloured and monochrome paths in a slightly inconsistent way (the colour mode ignores the pre-built `bar` variable). This is intentional — the colour mode builds inline — but the dead `_ = bar` should be removed and the logic simplified. Clean up during Task 4 or the next commit.
