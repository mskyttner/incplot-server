package main

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestHistBinCount(t *testing.T) {
	tests := []struct{ n, want int }{
		{5, 4},        // ceil(log2(5))+1  = 3+1 = 4
		{10, 5},       // ceil(log2(10))+1 = 4+1 = 5
		{50, 7},       // ceil(log2(50))+1 = 6+1 = 7
		{1000, 11},    // ceil(log2(1000))+1 = 10+1 = 11
		{1 << 20, 20}, // capped at 20
	}
	for _, tt := range tests {
		got := histBinCount(tt.n)
		if got != tt.want {
			t.Errorf("histBinCount(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestFiveNumber(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}
	minv, q1, med, q3, maxv := fiveNumber(data)
	// quantile(p=0.25) → idx=0.25*(5-1)=1.0 → data[1]=2
	// quantile(p=0.75) → idx=0.75*(5-1)=3.0 → data[3]=4
	if minv != 1 {
		t.Errorf("min: got %v want 1", minv)
	}
	if q1 != 2 {
		t.Errorf("q1: got %v want 2", q1)
	}
	if med != 3 {
		t.Errorf("median: got %v want 3", med)
	}
	if q3 != 4 {
		t.Errorf("q3: got %v want 4", q3)
	}
	if maxv != 5 {
		t.Errorf("max: got %v want 5", maxv)
	}
}

func TestQuantile(t *testing.T) {
	data := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	// p=0.0 → idx=0 → 0
	if got := quantile(data, 0.0); got != 0 {
		t.Errorf("p=0: got %v want 0", got)
	}
	// p=1.0 → idx=9 → 9
	if got := quantile(data, 1.0); got != 9 {
		t.Errorf("p=1: got %v want 9", got)
	}
	// p=0.5 → idx=4.5 → 4*(0.5)+5*(0.5) = 4.5
	if got := quantile(data, 0.5); got != 4.5 {
		t.Errorf("p=0.5: got %v want 4.5", got)
	}
}

func TestRenderHistSmoke(t *testing.T) {
	// 10 rows of value 1–10
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf(`{"v":%d}`, i))
	}
	ndjson := strings.Join(lines, "\n")
	schema := []colSchema{{Name: "v", ColType: "numeric"}}

	var sb strings.Builder
	if err := renderHist(&sb, schema, strings.NewReader(ndjson), 80, true); err != nil {
		t.Fatalf("renderHist: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "[") {
		t.Error("expected bracket labels in histogram output")
	}
	outLines := strings.Split(strings.TrimSpace(out), "\n")
	// Sturges for n=10: ceil(log2(10))+1 = 5
	if len(outLines) != 5 {
		t.Errorf("expected 5 bin rows for n=10, got %d:\n%s", len(outLines), out)
	}
}

func TestRenderHistColour(t *testing.T) {
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf(`{"v":%d}`, i))
	}
	schema := []colSchema{{Name: "v", ColType: "numeric"}}
	var sb strings.Builder
	if err := renderHist(&sb, schema, strings.NewReader(strings.Join(lines, "\n")), 80, false); err != nil {
		t.Fatalf("renderHist (colour): %v", err)
	}
	// Coloured output must contain ANSI escape
	if !strings.Contains(sb.String(), "\x1b[") {
		t.Error("expected ANSI escape codes in coloured histogram output")
	}
}

func TestRenderBoxSmoke(t *testing.T) {
	// 15 rows, three numeric columns: a, b, c
	var lines []string
	for i := 1; i <= 15; i++ {
		lines = append(lines, fmt.Sprintf(`{"a":%d,"b":%d,"c":%d}`, i, i*2, i*3))
	}
	raw := []byte(strings.Join(lines, "\n"))
	schema := []colSchema{
		{Name: "a", ColType: "numeric"},
		{Name: "b", ColType: "numeric"},
		{Name: "c", ColType: "numeric"},
	}
	var sb strings.Builder
	if err := renderBox(&sb, schema, raw, 80, true); err != nil {
		t.Fatalf("renderBox: %v", err)
	}
	out := sb.String()
	outLines := strings.Split(strings.TrimSpace(out), "\n")
	if len(outLines) != 3 {
		t.Errorf("expected 3 rows (one per column), got %d:\n%s", len(outLines), out)
	}
	// Each line should start with the column name
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
	// Box characters should be present
	if !strings.Contains(out, "=") {
		t.Error("expected '=' box characters in output")
	}
	if !strings.Contains(out, "|") {
		t.Error("expected '|' whisker/median characters in output")
	}
}

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
		t.Error("expected block fill character in bar output")
	}
	// 3 input rows → 3 output lines
	outLines := strings.Split(strings.TrimSpace(out), "\n")
	if len(outLines) != 3 {
		t.Errorf("expected 3 rows, got %d:\n%s", len(outLines), out)
	}
}

func TestRenderBarHMissingColumns(t *testing.T) {
	// Schema with no string column should return an error
	schema := []colSchema{{Name: "x", ColType: "numeric"}}
	err := renderBarH(io.Discard, schema, []byte(`{"x":1}`), 60, true)
	if err == nil {
		t.Error("expected error when no string column")
	}
}
