// server/gotui_render.go
package main

import (
	"encoding/base64"
	"encoding/json"
	_ "embed"
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

var playerIDSeq atomic.Int64

//go:embed assets/asciinema-player.min.js
var asciinemaPlayerJS []byte

//go:embed assets/asciinema-player.css
var asciinemaPlayerCSS []byte

// solLightPlayerTheme is the asciinema player custom theme matching solarized-light.
// palette: 16 ANSI colors (base02, red, green, yellow, blue, magenta, cyan, base2,
//          base03, orange, base01, base00, base0, violet, base1, base3).
const solLightPlayerTheme = `{background:"#fdf6e3",foreground:"#657b83",` +
	`palette:["#073642","#dc322f","#859900","#b58900","#268bd2","#d33682","#2aa198","#eee8d5",` +
	`"#002b36","#cb4b16","#586e75","#657b83","#839496","#6c71c4","#93a1a1","#fdf6e3"]}`

func ansiToAsciinemaHTML(ansi string, cols, rows int, fragment bool) string {
	castEvent, _ := json.Marshal(ansi)
	cast := fmt.Sprintf(
		`{"version":3,"term":{"cols":%d,"rows":%d},"title":"incplot"}`+"\n"+
			`[0.0,"o",%s]`+"\n",
		cols, rows, string(castEvent),
	)
	encoded := base64.StdEncoding.EncodeToString([]byte(cast))
	dataURL := "data:text/plain;base64," + encoded

	id := fmt.Sprintf("incplot-player-%d", playerIDSeq.Add(1))
	playerDiv := fmt.Sprintf(
		`<div id="%s" style="width:100%%"></div>`+
			`<script>%s</script>`+
			`<link rel="stylesheet" href="data:text/css;base64,%s">`+
			`<script>`+
			`AsciinemaPlayer.create(%q,document.getElementById(%q),`+
			`{cols:%d,rows:%d,controls:false,autoPlay:true,loop:false,theme:%s});`+
			`</script>`,
		id,
		string(asciinemaPlayerJS),
		base64.StdEncoding.EncodeToString(asciinemaPlayerCSS),
		dataURL, id, cols, rows, solLightPlayerTheme,
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

// bufToANSI converts a rendered gotui Buffer to an ANSI escape string.
// It walks cells row-by-row, emitting \x1b[38;2;R;G;Bm (fg) and
// \x1b[48;2;R;G;Bm (bg) only on style changes.
//
// Color encoding: ui.Color is tcell.Color (uint32).  The .RGB() method
// handles both direct RGB values (bit 30 set) and named palette colours
// (looked up via ColorValues).  It returns (-1,-1,-1) for ColorClear /
// ColorDefault, meaning "use the terminal default" — we skip escape codes
// in that case.
func bufToANSI(buf *ui.Buffer) string {
	rect := buf.Rectangle
	var sb strings.Builder

	type lastStyle struct{ fg, bg ui.Color }
	last := lastStyle{}

	emitColor := func(code int, c ui.Color) {
		r, g, b := c.RGB()
		if r < 0 {
			// ColorClear or invalid — do not emit an escape code.
			return
		}
		fmt.Fprintf(&sb, "\x1b[%d;2;%d;%d;%dm", code, r, g, b)
	}

	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			cell := buf.GetCell(image.Pt(x, y))
			fg := cell.Style.Fg
			bg := cell.Style.Bg
			if fg != last.fg {
				emitColor(38, fg)
				last.fg = fg
			}
			if bg != last.bg {
				emitColor(48, bg)
				last.bg = bg
			}
			r := cell.Rune
			if r == 0 {
				r = ' '
			}
			sb.WriteRune(r)
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("\x1b[0m")
	return sb.String()
}

// renderGotuiPlot is the entry point for heatmap/treemap/sparkline types.
// It reads NDJSON from src, builds the appropriate gotui widget, renders it
// to a buffer, converts to ANSI, and writes either text/plain or text/html.
func renderGotuiPlot(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	raw, err := io.ReadAll(src)
	if err != nil {
		http.Error(w, "read source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	ndjson, schema, err := toNDJSON(string(raw))
	if err != nil {
		http.Error(w, "parse data: "+err.Error(), http.StatusBadRequest)
		return
	}
	ndjsonBytes := []byte(ndjson)

	width, height := gotuiDims(opts)
	buf := ui.NewBuffer(image.Rect(0, 0, width, height))

	type drawable interface {
		SetRect(x1, y1, x2, y2 int)
		Draw(*ui.Buffer)
	}

	mono := opts.Mono

	var widget drawable
	switch opts.PlotType {
	case "heatmap":
		hm, e := heatmapWidget(ndjsonBytes, schema, mono)
		if e != nil {
			http.Error(w, e.Error(), http.StatusBadRequest)
			return
		}
		widget = hm
	case "treemap":
		tm, e := treemapWidget(ndjsonBytes, schema, mono)
		if e != nil {
			http.Error(w, e.Error(), http.StatusBadRequest)
			return
		}
		widget = tm
	case "sparkline":
		sg, e := sparklineWidget(ndjsonBytes, schema, mono)
		if e != nil {
			http.Error(w, e.Error(), http.StatusBadRequest)
			return
		}
		widget = sg
	default:
		http.Error(w, "unknown gotui plot type: "+opts.PlotType, http.StatusBadRequest)
		return
	}

	widget.SetRect(0, 0, width, height)
	widget.Draw(buf)

	// For HTML output, shrink the terminal to the actual content height so the
	// asciinema player isn't taller than the chart. Re-render at the trimmed
	// size so the player rows parameter matches.
	// Add 1 margin row: the heatmap widget reserves a virtual footer row for
	// X-labels that is not drawn when Border=false, but the layout still
	// accounts for it — without the +1 the re-render height is one row short
	// and the last data row is squeezed out.
	if opts.Format == "html" {
		if h := bufContentHeight(buf) + 1; h < height {
			height = h
			buf = ui.NewBuffer(image.Rect(0, 0, width, height))
			widget.SetRect(0, 0, width, height)
			widget.Draw(buf)
		}
	}

	ansi := bufToANSI(buf)

	// Color-mode widgets (heatmap, treemap, sparkline) express content via ANSI
	// background-color codes on space characters.  stripANSI leaves only spaces,
	// so the glyph check alone gives a false "empty" result.  Accept the output
	// if it has either text glyphs OR background-color escape sequences.
	if strings.TrimSpace(stripANSI(ansi)) == "" && !strings.Contains(ansi, "\x1b[48;") {
		http.Error(w, "renderer produced empty output", http.StatusInternalServerError)
		return
	}

	switch opts.Format {
	case "html":
		// The asciinema player's terminal emulator treats \n as line-feed only
		// (no implicit CR). Without \r the cursor drifts right by one column per
		// row, producing a staircase that wraps the last rows character-by-character.
		ansi = strings.ReplaceAll(ansi, "\n", "\r\n")
		// The \r\n after the last row scrolls a full terminal, pushing row 0
		// (the top border) off-screen. Strip just that final \r\n.
		ansi = strings.TrimSuffix(ansi, "\r\n\x1b[0m") + "\x1b[0m"
		// \x1b[?25l hides the blinking cursor block left at end of playback.
		ansi += "\x1b[?25l"
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, ansiToAsciinemaHTML(ansi, width, height, opts.Fragment))
	case "svg", "svg2":
		http.Error(w, "svg format not supported for "+opts.PlotType, http.StatusBadRequest)
		return
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, ansi)
	}
}

// gotuiDims returns terminal width and height for the given options.
// Height = round(width × 5/8) — golden ratio convention (80→50, 160→100).
func gotuiDims(opts RenderOptions) (w, h int) {
	w = 80
	if n, err := strconv.Atoi(opts.Width); err == nil && n >= 20 {
		w = n
	}
	if w < 20 {
		w = 20
	}
	h = int(math.Round(float64(w) * 5.0 / 8.0))
	if h < 10 {
		h = 10
	}
	return
}

// gotuiColors is the solarized palette used for multi-series plots.
var gotuiColors = []ui.Color{
	ui.NewRGBColor(133, 153, 0),   // solarized green
	ui.NewRGBColor(38, 139, 210),  // solarized blue
	ui.NewRGBColor(220, 50, 47),   // solarized red
	ui.NewRGBColor(181, 137, 0),   // solarized yellow
	ui.NewRGBColor(42, 161, 152),  // solarized cyan
	ui.NewRGBColor(108, 113, 196), // solarized violet
	ui.NewRGBColor(211, 54, 130),  // solarized magenta
	ui.NewRGBColor(203, 75, 22),   // solarized orange
}

// bufContentHeight scans the buffer from the last row upward and returns the
// 1-based index of the last row that has at least one "active" cell.  A cell
// is active when its rune is non-zero/non-space (text glyphs, shade chars) OR
// when it carries an explicit background color (color-mode heatmap fills cells
// with spaces that have colored backgrounds — checking only the rune would
// miss those rows and cause the trimmed height to be too short).
func bufContentHeight(buf *ui.Buffer) int {
	rect := buf.Rectangle
	for y := rect.Max.Y - 1; y >= rect.Min.Y; y-- {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			cell := buf.GetCell(image.Pt(x, y))
			r := cell.Rune
			if r != 0 && r != ' ' {
				return y - rect.Min.Y + 1
			}
			// rgb < 0 means ColorDefault/ColorClear — skip those.
			if ri, _, _ := cell.Style.Bg.RGB(); ri >= 0 {
				return y - rect.Min.Y + 1
			}
		}
	}
	return 1
}

// heatmapWidget builds a gotui Heatmap from NDJSON bytes + schema.
// All numeric columns → 2D value matrix; column names → X labels; row index → Y labels.
func heatmapWidget(raw []byte, schema []colSchema, mono bool) (*widgets.Heatmap, error) {
	names, colData := readAllNumericCols(raw, schema)
	if len(names) == 0 {
		return nil, fmt.Errorf("no numeric columns for heatmap")
	}
	rows := 0
	for _, col := range colData {
		if len(col) > rows {
			rows = len(col)
		}
	}
	if rows == 0 {
		return nil, fmt.Errorf("no data rows")
	}

	// Build row-major matrix: matrix[row][col]
	matrix := make([][]float64, rows)
	for r := 0; r < rows; r++ {
		matrix[r] = make([]float64, len(names))
		for c, col := range colData {
			if r < len(col) {
				matrix[r][c] = col[r]
			}
		}
	}

	yLabels := make([]string, rows)
	for i := range yLabels {
		yLabels[i] = strconv.Itoa(i + 1)
	}

	hm := widgets.NewHeatmap()
	hm.Border = false
	hm.Data = matrix
	hm.XLabels = names
	hm.YLabels = yLabels
	hm.MonochromeMode = mono
	return hm, nil
}

// treemapWidget builds a gotui TreeMap from NDJSON bytes + schema.
// First string column = labels; first numeric column = values.
// TreeMap uses a Root node with one child per label.
func treemapWidget(raw []byte, schema []colSchema, mono bool) (*widgets.TreeMap, error) {
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
		return nil, fmt.Errorf("treemap requires one string and one numeric column")
	}

	var children []*widgets.TreeMapNode
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		label := fmt.Sprint(row[labelCol])
		var val float64
		switch x := row[valueCol].(type) {
		case float64:
			val = x
		case string:
			val, _ = strconv.ParseFloat(x, 64)
		}
		children = append(children, &widgets.TreeMapNode{
			Label: label,
			Value: val,
			Style: ui.NewStyle(ui.ColorWhite, gotuiColors[len(children)%len(gotuiColors)]),
		})
	}
	if len(children) == 0 {
		return nil, fmt.Errorf("no data rows")
	}

	totalVal := 0.0
	for _, c := range children {
		totalVal += c.Value
	}
	root := &widgets.TreeMapNode{
		Label:    "root",
		Value:    totalVal,
		Children: children,
	}

	tm := widgets.NewTreeMap()
	tm.Border = false
	tm.Root = root
	tm.MonochromeMode = mono
	return tm, nil
}

// sparklineWidget builds a gotui SparklineGroup from NDJSON bytes + schema.
// One Sparkline per numeric column.
func sparklineWidget(raw []byte, schema []colSchema, mono bool) (*widgets.SparklineGroup, error) {
	names, colData := readAllNumericCols(raw, schema)
	if len(names) == 0 {
		return nil, fmt.Errorf("no numeric columns for sparkline")
	}

	var sparklines []*widgets.Sparkline
	for i, name := range names {
		sp := &widgets.Sparkline{
			Title:           name,
			Data:            colData[i],
			BackgroundColor: gotuiColors[i%len(gotuiColors)],
		}
		sparklines = append(sparklines, sp)
	}

	sg := widgets.NewSparklineGroup(sparklines...)
	sg.Border = false
	sg.MonochromeMode = mono
	return sg, nil
}
