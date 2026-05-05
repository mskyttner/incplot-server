// server/gotui_render.go
package main

import (
	"fmt"
	"image"
	"io"
	"net/http"
	"strings"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

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
// Full implementation in Task 10.
func renderGotuiPlot(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	http.Error(w, "gotui renderer not yet implemented", http.StatusNotImplemented)
	_ = (*widgets.SparklineGroup)(nil) // compile check — will be removed in Task 9
}
