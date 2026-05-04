package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

var (
	fontCSS     string
	fontCSSOnce sync.Once

	incplotFontCSSValue string
	incplotFontCSSOnce  sync.Once
)

// inlineFontCSS returns @font-face blocks with fonts base64-encoded as data
// URIs. Results are cached after the first call. If font files are missing the
// returned string is empty and the browser falls back to monospace.
func inlineFontCSS() string {
	fontCSSOnce.Do(func() { fontCSS = buildFontCSS() })
	return fontCSS
}

// incplotFontCSS returns the @font-face block that incplot embeds in its own
// HTML output (a minified woff2 subset), cached after the first call.
// Returns empty string if incplot cannot be invoked.
func incplotFontCSS() string {
	incplotFontCSSOnce.Do(func() { incplotFontCSSValue = extractIncplotFontCSS() })
	return incplotFontCSSValue
}

func extractIncplotFontCSS() string {
	// Run incplot --html with a tiny hardcoded CSV (no duckdb needed).
	cmd := newCommand(incplotBinary, "--font", defaultFont, "-w", "40", "--html")
	cmd.Stdin = strings.NewReader("x,y\n1,2\n2,4\n3,9\n4,16\n5,25\n")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	html := out.String()
	// Extract the @font-face block from the <style> section.
	start := strings.Index(html, "@font-face")
	end := strings.Index(html, "\n}")
	if start < 0 || end < start {
		return ""
	}
	return html[start:end+2] + "\n"
}

// svgMetricsCache maps font family name → svgFontMetrics.
var svgMetricsCache sync.Map

type svgFontMetrics struct {
	LineHeight  float32
	AdvanceEm   float32 // advance of '0' in em units (0 → unknown)
}

// svgMetrics returns the cached font metrics for the given family name,
// computing them via fc-match + sfnt on first call.
func svgMetrics(fontName string) svgFontMetrics {
	if v, ok := svgMetricsCache.Load(fontName); ok {
		return v.(svgFontMetrics)
	}
	m := computeSVGMetrics(fontName)
	svgMetricsCache.Store(fontName, m)
	return m
}

// svgLineHeight returns the correct ansisvg LineHeight for the given font.
func svgLineHeight(fontName string) float32 {
	return svgMetrics(fontName).LineHeight
}

// svgCharWidthPx returns the explicit column width in pixels for the given
// font and font size (ceil of advance_em × fontSize). Returns 0 if the
// advance cannot be determined, in which case ansisvg falls back to ch units.
func svgCharWidthPx(fontName string, fontSize int) int {
	adv := svgMetrics(fontName).AdvanceEm
	if adv <= 0 {
		return 0
	}
	// math.Ceil rounds up so glyphs never get clipped on the right.
	return int(math.Ceil(float64(adv) * float64(fontSize)))
}

func computeSVGMetrics(fontName string) svgFontMetrics {
	const fallbackLH = float32(1.25)

	// fc-match resolves a family name to the best-matching font file.
	out, err := exec.Command("fc-match", "--format=%{file}", fontName).Output()
	if err != nil {
		return svgFontMetrics{LineHeight: fallbackLH}
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return svgFontMetrics{LineHeight: fallbackLH}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return svgFontMetrics{LineHeight: fallbackLH}
	}
	f, err := sfnt.Parse(data)
	if err != nil {
		return svgFontMetrics{LineHeight: fallbackLH}
	}

	// Use a large ppem so fixed-point rounding errors are negligible.
	var buf sfnt.Buffer
	const ppem = fixed.Int26_6(1000 << 6)

	// Box-drawing │ (U+2502) spans the full cell top-to-bottom and must tile
	// seamlessly between rows, so its glyph bounds set the required LineHeight.
	// sfnt.Metrics() uses the hhea table which can differ from OS/2 sTypo
	// metrics (e.g. Adwaita Mono hhea descent=-215 vs OS/2 descent=-285).
	lh := fallbackLH
	if gi, err := f.GlyphIndex(&buf, 0x2502); err == nil && gi != 0 {
		if bounds, _, err := f.GlyphBounds(&buf, gi, ppem, font.HintingNone); err == nil {
			// bounds.Max.Y = ascender (positive), bounds.Min.Y = descender (negative)
			v := float32(bounds.Max.Y-bounds.Min.Y) / float32(ppem)
			if v >= 1.0 {
				lh = v
			}
		}
	} else {
		// Fallback: use ascent+descent from font metrics (hhea table).
		if m, err := f.Metrics(&buf, ppem, font.HintingNone); err == nil {
			v := float32(m.Ascent+m.Descent) / float32(ppem)
			if v >= 1.0 {
				lh = v
			}
		}
	}

	// Advance width of '0' — canonical monospace width character.
	// Used to compute explicit px column positions in the SVG, eliminating
	// dependence on the browser's `ch` unit which may be computed from the
	// fallback font before the embedded woff2 finishes loading.
	var advanceEm float32
	if gi, err := f.GlyphIndex(&buf, '0'); err == nil && gi != 0 {
		if adv, err := f.GlyphAdvance(&buf, gi, ppem, font.HintingNone); err == nil {
			advanceEm = float32(adv) / float32(ppem)
		}
	}

	return svgFontMetrics{LineHeight: lh, AdvanceEm: advanceEm}
}

func buildFontCSS() string {
	dir := os.Getenv("ADWAITA_FONT_DIR")
	if dir == "" {
		dir = "/usr/local/share/fonts/adwaita-mono"
	}

	type face struct{ weight, style, file string }
	faces := []face{
		{"400", "normal", "AdwaitaMono-Regular.ttf"},
		{"700", "normal", "AdwaitaMono-Bold.ttf"},
		{"400", "italic", "AdwaitaMono-Italic.ttf"},
	}

	var css string
	for _, f := range faces {
		data, err := os.ReadFile(dir + "/" + f.file)
		if err != nil {
			continue
		}
		css += fmt.Sprintf(
			"@font-face {\n"+
				"  font-family: \"Adwaita Mono\";\n"+
				"  font-weight: %s; font-style: %s;\n"+
				"  src: url(\"data:font/truetype;base64,%s\");\n"+
				"}\n",
			f.weight, f.style, base64.StdEncoding.EncodeToString(data),
		)
	}
	return css
}
