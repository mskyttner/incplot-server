package main

import (
	"fmt"
	gohtml "html"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// h2sFontSize matches ansitosvg's DefaultOptions.FontSize so both SVG paths
	// produce the same font size.
	h2sFontSize = 14.0
)

var (
	// h2sSpanRe matches <span style="color:#rrggbb;">…</span>.
	// Span content never contains raw '<' because incplot HTML-escapes it.
	h2sSpanRe = regexp.MustCompile(`<span style="color:(#[0-9a-fA-F]{6});">(.*?)</span>`)

	// h2sFontFaceRe extracts the @font-face block from the <style> section.
	// base64 data never contains '}', so [^}]+ is safe here.
	h2sFontFaceRe = regexp.MustCompile(`@font-face\s*\{[^}]+\}`)

	// h2sBgColorRe finds the background-color in .term-output CSS.
	h2sBgColorRe = regexp.MustCompile(`background-color:\s*(#[0-9a-fA-F]{6})`)

	// h2sFgColorRe finds the default text color (color: not background-color:).
	// [^-] before "color:" excludes "background-color:".
	h2sFgColorRe = regexp.MustCompile(`[^-]color:\s*(#[0-9a-fA-F]{6})`)
)

type colorSpan struct {
	text  string // HTML-unescaped content
	color string // "#rrggbb"
}

// htmlToSVG converts the full HTML output of `incplot --html` to an SVG image.
//
// charWidthPx and lineHeightPx must be the real measured font metrics for the
// chosen font at fontSize — the same values the ansitosvg path derives via
// svgCharWidthPx / svgLineHeight in fonts.go.  Using measured values (rather
// than the WOFF2's normalised 0.5 em advance) ensures the SVG canvas is sized
// correctly in all renderers, not only browsers that load the embedded font.
func htmlToSVG(src string, fontSize, charWidthPx, lineHeightPx float64) (string, error) {
	// Mirror the padding used by the ansitosvg path (MarginSize.X = charWidth/2, Y = 0).
	padX := charWidthPx * 0.5
	padY := 0.0

	// ── font face ────────────────────────────────────────────────────────────
	fontFaceCSS := ""
	if m := h2sFontFaceRe.FindString(src); m != "" {
		// Fix MIME type: incplot emits "data:font/woff" but the bytes are WOFF2.
		fontFaceCSS = strings.ReplaceAll(m, "data:font/woff;", "data:font/woff2;")
	}

	// ── colors ───────────────────────────────────────────────────────────────
	bgColor := "#000000"
	if m := h2sBgColorRe.FindStringSubmatch(src); len(m) > 1 {
		bgColor = m[1]
	}
	fgColor := "#cccccc"
	if m := h2sFgColorRe.FindStringSubmatch(src); len(m) > 1 {
		fgColor = m[1]
	}

	// ── pre content ──────────────────────────────────────────────────────────
	// The <pre> content starts with \n and ends with \n, matching the leading
	// and trailing blank lines that the ANSI stream gives ansitosvg.  Keep
	// exactly one of each so NrLines and canvas height align with the old path.
	// Strip diagnostic lines (warnings incplot appends after the plot) before
	// measuring or rendering — they are plain text with no <span> tags and must
	// not contribute to maxCols or appear as SVG text elements.
	preContent := between(src, `<pre class="term-output">`, `</pre>`)
	preContent = strings.TrimRight(stripIncplotDiagnostics(preContent), "\n") + "\n"

	// ── parse lines ──────────────────────────────────────────────────────────
	rawLines := strings.Split(preContent, "\n")
	parsed := make([][]colorSpan, len(rawLines))
	maxCols := 0
	for i, rl := range rawLines {
		parsed[i] = h2sParseHTMLLine(rl, fgColor)
		cols := 0
		for _, s := range parsed[i] {
			cols += utf8.RuneCountInString(s.text)
		}
		if cols > maxCols {
			maxCols = cols
		}
	}

	// ── SVG dimensions ───────────────────────────────────────────────────────
	svgW := math.Ceil(padX*2 + float64(maxCols)*charWidthPx)
	svgH := math.Ceil(padY*2 + float64(len(parsed))*lineHeightPx)

	// ── build SVG ────────────────────────────────────────────────────────────
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f">`+"\n", svgW, svgH)

	if fontFaceCSS != "" {
		b.WriteString("  <defs><style>\n    ")
		b.WriteString(fontFaceCSS)
		b.WriteString("\n  </style></defs>\n")
	}

	fmt.Fprintf(&b, `  <rect width="100%%" height="100%%" fill="%s"/>`+"\n", bgColor)
	fmt.Fprintf(&b, `  <g font-family="incplot_minified_font, monospace" font-size="%.1f">`+"\n", fontSize)

	for i, line := range parsed {
		hasContent := false
		for _, s := range line {
			if s.text != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			continue
		}

		// y is the text baseline, mirroring ansitosvg's rowCoordinate(row+0.5):
		// place baseline at the centre of each character cell.
		y := padY + (float64(i)+0.5)*lineHeightPx
		fmt.Fprintf(&b, "    <text y=\"%.2f\">", y)

		xCol := 0
		for _, s := range line {
			if s.text == "" {
				continue
			}
			x := padX + float64(xCol)*charWidthPx
			fmt.Fprintf(&b, `<tspan x="%.2f" fill="%s">%s</tspan>`,
				x, s.color, gohtml.EscapeString(s.text))
			xCol += utf8.RuneCountInString(s.text)
		}
		b.WriteString("</text>\n")
	}

	b.WriteString("  </g>\n</svg>\n")
	return b.String(), nil
}

// h2sParseHTMLLine splits one line of incplot HTML output into colorSpan
// segments.  Text between spans inherits defaultColor.
func h2sParseHTMLLine(line, defaultColor string) []colorSpan {
	var out []colorSpan
	pos := 0
	for _, loc := range h2sSpanRe.FindAllStringSubmatchIndex(line, -1) {
		// Plain text before this span.
		if loc[0] > pos {
			if t := gohtml.UnescapeString(line[pos:loc[0]]); t != "" {
				out = append(out, colorSpan{t, defaultColor})
			}
		}
		color := line[loc[2]:loc[3]]
		text := gohtml.UnescapeString(line[loc[4]:loc[5]])
		if text != "" {
			out = append(out, colorSpan{text, color})
		}
		pos = loc[1]
	}
	// Plain text after the last span.
	if pos < len(line) {
		if t := gohtml.UnescapeString(line[pos:]); t != "" {
			out = append(out, colorSpan{t, defaultColor})
		}
	}
	return out
}
