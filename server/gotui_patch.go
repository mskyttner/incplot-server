// server/gotui_patch.go
//
// MonochromeMode patches applied to vendored gotui v5.0.3 widgets:
//
//   vendor/github.com/metaspartan/gotui/v5/widgets/heatmap.go   — MonochromeMode bool (pre-existing patch)
//   vendor/github.com/metaspartan/gotui/v5/widgets/treemap.go   — MonochromeMode bool (pre-existing patch)
//   vendor/github.com/metaspartan/gotui/v5/widgets/sparkline.go — MonochromeMode bool (added in Task 7)
//
// When MonochromeMode=true each widget uses glyph fills on the default background
// instead of coloured space characters, making output readable without ANSI colour
// support (email, plain-text terminals, MCP result boxes).
//
// Heatmap/TreeMap: use SHADED_BLOCKS density glyphs (' ', '░', '▒', '▓', '█') on
// default background instead of space+coloured-background cells.
//
// SparklineGroup: use BARS height glyphs ('▁'–'█') on ui.ColorClear (default)
// background instead of glyph+lineColor+BackgroundColor cells.
//
// Re-apply if vendor/ is regenerated with go mod vendor.
package main
