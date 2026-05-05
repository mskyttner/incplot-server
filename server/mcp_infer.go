package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

type colSchema struct {
	Name    string
	ColType string // "numeric", "string", or "temporal"
}

// toNDJSON detects whether data is CSV or NDJSON, converts if needed, and
// returns NDJSON plus column schema derived from up to five sample rows.
func toNDJSON(data string) (string, []colSchema, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return "", nil, fmt.Errorf("empty data")
	}
	first := strings.TrimSpace(strings.SplitN(data, "\n", 2)[0])
	if strings.HasPrefix(first, "{") {
		// Already NDJSON — ensure trailing newline for incplot.
		return data + "\n", schemaFromNDJSON(data), nil
	}
	return csvToNDJSON(data)
}

func csvToNDJSON(data string) (string, []colSchema, error) {
	r := csv.NewReader(strings.NewReader(data))
	r.TrimLeadingSpace = true
	headers, err := r.Read()
	if err != nil {
		return "", nil, fmt.Errorf("CSV header: %w", err)
	}
	var sample [][]string
	var sb strings.Builder
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("CSV row: %w", err)
		}
		if len(sample) < 5 {
			sample = append(sample, row)
		}
		sb.WriteString(rowToJSON(headers, row))
		sb.WriteByte('\n')
	}
	return sb.String(), inferSchemaFromSample(headers, sample), nil
}

func rowToJSON(headers, values []string) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, h := range headers {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(h)
		b.Write(key)
		b.WriteByte(':')
		v := ""
		if i < len(values) {
			v = values[i]
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			b.WriteString(strconv.FormatFloat(f, 'f', -1, 64))
		} else {
			quoted, _ := json.Marshal(v)
			b.Write(quoted)
		}
	}
	b.WriteByte('}')
	return b.String()
}

// schemaFromNDJSON infers schema from the first five NDJSON rows.
// Key order is non-deterministic (Go map) but that is fine — type inference
// only counts column types, not their order.
func schemaFromNDJSON(data string) []colSchema {
	var sample []map[string]any
	for i, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if i >= 5 {
			break
		}
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) == nil {
			sample = append(sample, row)
		}
	}
	if len(sample) == 0 {
		return nil
	}
	var keys []string
	seen := map[string]bool{}
	for k := range sample[0] {
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	schema := make([]colSchema, len(keys))
	for i, k := range keys {
		vals := make([]string, 0, len(sample))
		for _, row := range sample {
			vals = append(vals, fmt.Sprint(row[k]))
		}
		schema[i] = colSchema{Name: k, ColType: inferColType(k, vals)}
	}
	return schema
}

func inferSchemaFromSample(headers []string, rows [][]string) []colSchema {
	schema := make([]colSchema, len(headers))
	for i, h := range headers {
		vals := make([]string, 0, len(rows))
		for _, row := range rows {
			if i < len(row) {
				vals = append(vals, row[i])
			}
		}
		schema[i] = colSchema{Name: h, ColType: inferColType(h, vals)}
	}
	return schema
}

var temporalKeywords = []string{
	"year", "date", "time", "month", "week", "day", "quarter", "period",
}

func inferColType(name string, values []string) string {
	lower := strings.ToLower(name)
	for _, kw := range temporalKeywords {
		if strings.Contains(lower, kw) {
			return "temporal"
		}
	}
	allYears := true
	allNumeric := true
	for _, v := range values {
		if v == "" || v == "null" {
			continue
		}
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			allNumeric = false
			allYears = false
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1800 || n > 2100 {
			allYears = false
		}
	}
	if allYears && len(values) > 0 {
		return "temporal"
	}
	if allNumeric {
		return "numeric"
	}
	return "string"
}

// inferPlotType maps column-type counts and row count to the best chart type.
// Evaluated top-to-bottom; more specific / data-rich rules come first.
//
//	S=0, T=0, N=1,  rows≥5  → hist      (textchart)
//	S=0, T=0, N≥2,  rows≥10 → box       (textchart)
//	S=0, T=0, N≥3,  rows≥3  → heatmap   (gotui)
//	S=1, N=1,       rows≥10 → treemap   (gotui)
//	T≥1, N≥4               → sparkline (gotui)
//	T≥1, N≥1               → line      (incplot)
//	S=1, N=1               → barV      (incplot)
//	S=1, N=2..3            → barVM     (incplot)
//	S=1, N≥4              → barHS     (incplot)
//	S=0, T=0, N=2          → scatter   (incplot)
//	fallback               → line
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
