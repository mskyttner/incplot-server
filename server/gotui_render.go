// server/gotui_render.go
package main

import (
	"io"
	"net/http"

	ui "github.com/metaspartan/gotui/v5"
)

// renderGotuiPlot is the entry point for heatmap/treemap/sparkline types.
// Full implementation in Task 10.
func renderGotuiPlot(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	http.Error(w, "gotui renderer not yet implemented", http.StatusNotImplemented)
	_ = ui.ColorClear // confirm import resolves
}
