package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type chartPalette struct {
	Series [][3]int
	Axis   [3]int
	Label  [3]int
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

// readAllNumericCols returns names and per-column float64 slices for every
// numeric column in schema order, reading from raw NDJSON bytes.
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

// renderTextChart is the HTTP entry point for hist/box/barH types.
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

	var sb strings.Builder
	switch opts.PlotType {
	case "hist":
		err = renderHist(&sb, schema, strings.NewReader(string(raw)), widthInt, opts.Format == "html")
	case "box":
		err = renderBox(&sb, schema, raw, widthInt, opts.Format == "html")
	case "barH":
		err = renderBarH(&sb, schema, raw, widthInt, opts.Format == "html")
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
		css := `<style>body{background:#fdf6e3;margin:0}` +
			`pre{font-family:"Adwaita Mono","JetBrains Mono",monospace;` +
			`font-size:14px;line-height:1.4;padding:16px;white-space:pre}</style>`
		if opts.Fragment {
			fmt.Fprintf(w, "%s<pre>%s</pre>", css, html.EscapeString(body))
		} else {
			fmt.Fprintf(w,
				`<!DOCTYPE html><html><head><meta charset="utf-8">%s</head>`+
					`<body><pre>%s</pre></body></html>`, css, html.EscapeString(body))
		}
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, body)
	}
}

// renderHist writes a histogram of the first numeric column.
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

// renderBox writes one box-plot row per numeric column.
// One row per column: label, then a box plot scaled to plotW characters.
func renderBox(w io.Writer, schema []colSchema, raw []byte, width int, mono bool) error {
	names, colData := readAllNumericCols(raw, schema)
	if len(names) == 0 {
		return fmt.Errorf("no numeric columns for box plot")
	}

	// Compute global axis span across all columns.
	globalMin, globalMax := math.Inf(1), math.Inf(-1)
	type fn5 struct{ mn, q1, med, q3, mx float64 }
	fiveNums := make([]fn5, len(names))
	for i, data := range colData {
		if len(data) == 0 {
			continue
		}
		mn, q1, med, q3, mx := fiveNumber(append([]float64{}, data...))
		fiveNums[i] = fn5{mn, q1, med, q3, mx}
		if mn < globalMin {
			globalMin = mn
		}
		if mx > globalMax {
			globalMax = mx
		}
	}
	if math.IsInf(globalMin, 1) {
		return fmt.Errorf("no data rows")
	}
	span := globalMax - globalMin
	if span == 0 {
		span = 1
	}

	// Label column width: max column name length + 2 spaces.
	labelW := 0
	for _, n := range names {
		if len(n) > labelW {
			labelW = len(n)
		}
	}
	labelW += 2

	plotW := width - labelW - 14 // 14 chars for median label on right
	if plotW < 20 {
		plotW = 20
	}

	scale := func(v float64) int {
		pos := int(math.Round((v - globalMin) / span * float64(plotW-1)))
		if pos < 0 {
			pos = 0
		}
		if pos >= plotW {
			pos = plotW - 1
		}
		return pos
	}

	green := solLight.Series[0]
	label_color := solLight.Label

	for i, name := range names {
		fn := fiveNums[i]
		pMn := scale(fn.mn)
		pQ1 := scale(fn.q1)
		pMed := scale(fn.med)
		pQ3 := scale(fn.q3)
		pMx := scale(fn.mx)

		// Build the box string: spaces, whiskers (-), box (=), median (|).
		row := make([]byte, plotW)
		for j := range row {
			row[j] = ' '
		}
		for j := pMn; j <= pMx; j++ {
			row[j] = '-'
		}
		for j := pQ1; j <= pQ3; j++ {
			row[j] = '='
		}
		row[pMn] = '|'
		row[pMx] = '|'
		row[pMed] = '|'

		label := fmt.Sprintf("%-*s", labelW, name)
		medLabel := fmt.Sprintf("  %g med", fn.med)
		if !mono {
			fmt.Fprintf(w, "%s%s%s%s%s%s%s\n",
				tcFg(label_color[0], label_color[1], label_color[2]), label, tcReset(),
				tcFg(green[0], green[1], green[2]), string(row), tcReset(),
				medLabel)
		} else {
			fmt.Fprintf(w, "%-*s%s%s\n", labelW, name, string(row), medLabel)
		}
	}
	return nil
}

// renderBarH writes a horizontal text bar chart.
// Requires the first string column (labels) and first numeric column (values).
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

	type barRow struct {
		label string
		value float64
	}
	var rows []barRow
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
		rows = append(rows, barRow{label, val})
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
	axis  := solLight.Axis
	lbl   := solLight.Label
	for _, r := range rows {
		filled := int(math.Round(r.value / maxVal * float64(barW)))
		empty  := barW - filled
		truncLabel := r.label
		if len(truncLabel) > labelW { truncLabel = truncLabel[:labelW] }
		if !mono {
			barStr := tcFg(green[0], green[1], green[2]) + strings.Repeat("█", filled) + tcReset() +
				tcFg(axis[0], axis[1], axis[2]) + strings.Repeat("░", empty) + tcReset()
			fmt.Fprintf(w, "%s%-*s%s  %s  %.4g\n",
				tcFg(lbl[0], lbl[1], lbl[2]), labelW, truncLabel, tcReset(),
				barStr,
				r.value)
		} else {
			fmt.Fprintf(w, "%-*s  %s%s  %.4g\n",
				labelW, truncLabel,
				strings.Repeat("█", filled),
				strings.Repeat("░", empty),
				r.value)
		}
	}
	return nil
}
