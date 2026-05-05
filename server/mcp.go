package main

import (
	"encoding/json"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newMCPHandler() http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: "incplot", Version: "v1"}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "plot",
		Description: "Render tabular data as a text/ANSI chart. Data must be CSV with a header row or newline-delimited JSON objects. Omit type to auto-select from data shape. Returns ANSI-colored text by default (raw=true); pass raw=false for plain monochrome text that renders correctly in any context. Note: when called from Claude Code via this MCP connection, tool result boxes do not render ANSI — for colored output use the HTTP endpoint directly: curl 'http://localhost:8080/incplot/plot?source=URL&format=text' heatmap/treemap/sparkline are rendered by gotui; hist/box/barH by a pure-Go renderer; all other types use the incplot binary. barH is explicit-only and not auto-inferred.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["data"],
			"properties": {
				"data":  {"type": "string",  "description": "CSV with header row, or newline-delimited JSON objects"},
				"type":  {"type": "string",  "description": "Chart type; omit to auto-select", "enum": ["line","scatter","barV","barHS","barHM","barVM",
				         "heatmap","treemap","sparkline",
				         "hist","box","barH"]},
				"width": {"type": "integer", "description": "Output width in characters (default 80)", "minimum": 40, "maximum": 400},
				"raw":   {"type": "boolean", "description": "Include ANSI color codes (default true). Set false for plain monochrome text — use when the display context cannot render ANSI, e.g. Claude Code MCP result boxes."}
			}
		}`),
	}, mcpPlotHandler)

	srv.AddTool(&mcp.Tool{
		Name:        "source_plot",
		Description: "Render a named built-in data source as a text/ANSI chart. Use list_sources to see available source names. Returns ANSI-colored text by default (raw=true); pass raw=false for plain monochrome text. Note: when called from Claude Code via this MCP connection, tool result boxes do not render ANSI — for colored output use the HTTP endpoint directly: curl 'http://localhost:8080/incplot/plot?source=http://localhost:8080/incplot/source/NAME&format=text' heatmap/treemap/sparkline are rendered by gotui; hist/box/barH by a pure-Go renderer; all other types use the incplot binary. barH is explicit-only and not auto-inferred.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["source"],
			"properties": {
				"source": {"type": "string",  "description": "Source name, e.g. german_economy"},
				"type":   {"type": "string",  "description": "Chart type; omit to use the source default", "enum": ["line","scatter","barV","barHS","barHM","barVM",
				          "heatmap","treemap","sparkline",
				          "hist","box","barH"]},
				"width":  {"type": "integer", "description": "Output width in characters (default 80)", "minimum": 40, "maximum": 400},
				"raw":    {"type": "boolean", "description": "Include ANSI color codes (default true). Set false for plain monochrome text — use when the display context cannot render ANSI, e.g. Claude Code MCP result boxes."}
			}
		}`),
	}, mcpSourcePlotHandler)

	srv.AddTool(&mcp.Tool{
		Name:        "list_sources",
		Description: "List available named data sources with their descriptions and default chart types.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, mcpListSourcesHandler)

	return mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return srv }, nil)
}
