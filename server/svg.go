package main

import (
	"fmt"
	"strings"
)

// injectSVGFonts inlines the @font-face block from incplot's own HTML output
// into the SVG's <style> block and rewrites the font-family reference to match.
func injectSVGFonts(svg string) string {
	css := incplotFontCSS()
	if css == "" {
		return svg
	}
	// Fix MIME type: incplot embeds the font with data:font/woff MIME type but
	// the data is actually woff2 (magic bytes wOF2). Some SVG renderers reject
	// the font if the MIME type doesn't match, falling back to the system font.
	css = strings.ReplaceAll(css, "data:font/woff;", "data:font/woff2;")

	const styleMarker = "<style>"
	idx := strings.Index(svg, styleMarker)
	if idx < 0 {
		return svg
	}
	insert := idx + len(styleMarker)
	svg = svg[:insert] + "\n" + css + svg[insert:]

	svg = strings.ReplaceAll(svg, "font-family: "+defaultFont+", monospace",
		"font-family: incplot_minified_font, monospace")
	return svg
}

// injectSVGBackground inserts a background <rect> with the theme colour
// immediately after the SVG root element's opening tag.
func injectSVGBackground(svg, theme string) string {
	bg, ok := themeBG[theme]
	if !ok {
		return svg
	}
	rect := fmt.Sprintf(
		`<rect width="100%%" height="100%%" x="0" y="0" style="fill: rgb(%d,%d,%d)"/>`,
		bg[0], bg[1], bg[2],
	)
	const marker = `xml:space="preserve">`
	idx := strings.Index(svg, marker)
	if idx < 0 {
		return svg
	}
	insert := idx + len(marker)
	return svg[:insert] + "\n    " + rect + svg[insert:]
}
