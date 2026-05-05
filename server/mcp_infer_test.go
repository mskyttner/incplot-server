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
		{"hist_too_few",   cols("numeric"), 4,  "line"},

		// New: box (multi-numeric, ≥10 rows)
		{"box_two",         cols("numeric", "numeric"), 10, "box"},
		{"box_three",       cols("numeric", "numeric", "numeric"), 10, "box"},
		{"box_too_few",     cols("numeric", "numeric"), 9,  "scatter"},

		// New: heatmap (≥3 numeric, ≥3 rows, no S/T)
		{"heatmap_basic",   cols("numeric", "numeric", "numeric"), 5,  "heatmap"},
		{"heatmap_min",     cols("numeric", "numeric", "numeric"), 3,  "heatmap"},
		{"heatmap_few",     cols("numeric", "numeric", "numeric"), 2,  "line"},

		// New: treemap (S=1, N=1, ≥10 rows)
		{"treemap_basic",   []colSchema{{ColType: "string"}, {ColType: "numeric"}}, 10, "treemap"},
		{"treemap_too_few", []colSchema{{ColType: "string"}, {ColType: "numeric"}}, 9,  "barV"},

		// New: sparkline (T≥1, N≥4)
		{"sparkline_basic", cols("temporal", "numeric", "numeric", "numeric", "numeric"), 5, "sparkline"},
		{"sparkline_exact", cols("temporal", "numeric", "numeric", "numeric", "numeric"), 1, "sparkline"},
		{"sparkline_n3",    cols("temporal", "numeric", "numeric", "numeric"), 5, "line"},

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
