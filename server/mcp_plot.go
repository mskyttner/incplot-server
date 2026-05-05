package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func mcpPlotHandler(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Data  string `json:"data"`
		Type  string `json:"type"`
		Width int    `json:"width"`
		Raw   *bool  `json:"raw"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return mcpErr("invalid arguments: " + err.Error()), nil
	}
	if strings.TrimSpace(args.Data) == "" {
		return mcpErr("data is required"), nil
	}

	width := 80
	if args.Width >= 40 && args.Width <= 400 {
		width = args.Width
	}

	ndjson, schema, err := toNDJSON(args.Data)
	if err != nil {
		return mcpErr("data parse error: " + err.Error()), nil
	}

	plotType := args.Type
	if plotType == "" {
		rowCount := strings.Count(strings.TrimRight(ndjson, "\n"), "\n") + 1
		plotType = inferPlotType(schema, rowCount)
	}

	rec := httptest.NewRecorder()
	renderPlotText(rec, strings.NewReader(ndjson), RenderOptions{
		Format:   "text",
		PlotType: plotType,
		Width:    strconv.Itoa(width),
		Theme:    defaultTheme,
	})
	if rec.Code != http.StatusOK {
		return mcpErr(strings.TrimSpace(rec.Body.String())), nil
	}
	out := rec.Body.String()
	if args.Raw != nil && !*args.Raw {
		out = stripANSI(out)
	}
	return mcpText(out), nil
}

func mcpSourcePlotHandler(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Source string `json:"source"`
		Type   string `json:"type"`
		Width  int    `json:"width"`
		Raw    *bool  `json:"raw"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return mcpErr("invalid arguments: " + err.Error()), nil
	}
	if strings.TrimSpace(args.Source) == "" {
		return mcpErr("source is required"), nil
	}

	width := 80
	if args.Width >= 40 && args.Width <= 400 {
		width = args.Width
	}

	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	renderFromSource(rec, r, RenderOptions{
		SourceURL: basePath + "/source/" + args.Source,
		Format:    "text",
		PlotType:  args.Type,
		Width:     strconv.Itoa(width),
		Theme:     defaultTheme,
	})
	if rec.Code != http.StatusOK {
		return mcpErr(strings.TrimSpace(rec.Body.String())), nil
	}
	out := rec.Body.String()
	if args.Raw != nil && !*args.Raw {
		out = stripANSI(out)
	}
	return mcpText(out), nil
}

func mcpListSourcesHandler(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-20s %-13s %s\n", "name", "default_type", "description"))
	sb.WriteString(strings.Repeat("─", 70) + "\n")
	for _, s := range builtinSources {
		sb.WriteString(fmt.Sprintf("%-20s %-13s %s\n", s.Name, s.DefaultType, s.Description))
	}
	sourceRegistry.Range(func(_, v any) bool {
		s := v.(SourceMacro)
		if s.DataFile == "" { // dynamic sources only — builtins already listed above
			sb.WriteString(fmt.Sprintf("%-20s %-13s %s\n", s.Name, s.DefaultType, s.Label))
		}
		return true
	})
	return mcpText(sb.String()), nil
}

func mcpText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func mcpErr(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
}
