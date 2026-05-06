package main

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/wader/ansisvg/ansitosvg"
)

// RenderOptions holds all parameters controlling how a source is rendered.
type RenderOptions struct {
	SourceURL string
	Format    string // "html" | "svg" | "text"; default "html"
	Fragment  bool   // html only: return body fragment for HTMX insertion
	Width     string
	Font      string
	Theme     string
	Canvas    bool   // html only: use --html-canvas instead of --html
	PlotType  string // "line" | "scatter" | "barV" | "barHS" | "barHM" | "barVM"
	Mono      bool   // use shade-block glyphs instead of space+bg-colour (for plain-text consumers)
}

func parseRenderOptions(q interface{ Get(string) string }) RenderOptions {
	return RenderOptions{
		SourceURL: q.Get("source"),
		Format:    orDefault(q.Get("format"), "html"),
		Fragment:  q.Get("fragment") == "1",
		Width:     orDefault(q.Get("width"), defaultWidth),
		Font:      orDefault(q.Get("font"), envDefaultFont),
		Theme:     orDefault(q.Get("theme"), defaultTheme),
		Canvas:    q.Get("canvas") == "1",
		PlotType:  q.Get("type"),
		Mono:      q.Get("mono") == "1",
	}
}

// plotHandler serves GET /incplot/plot — full HTML page, SVG image, or plain text.
func plotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	opts := parseRenderOptions(r.URL.Query())
	if strings.TrimSpace(opts.SourceURL) == "" {
		http.Error(w, "source query parameter is required", http.StatusBadRequest)
		return
	}
	renderFromSource(w, r, opts)
}

// renderHandler serves GET /incplot/render — always returns an HTML fragment
// for HTMX live preview in the UI.
func renderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	opts := parseRenderOptions(r.URL.Query())
	opts.Format = "html"
	opts.Fragment = true
	if strings.TrimSpace(opts.SourceURL) == "" {
		http.Error(w, "source query parameter is required", http.StatusBadRequest)
		return
	}
	renderFromSource(w, r, opts)
}

// renderFromSource resolves the source URL, fetches its NDJSON stream, and renders it.
func renderFromSource(w http.ResponseWriter, r *http.Request, opts RenderOptions) {
	srcURL := opts.SourceURL

	// For server-local paths, route through the mux in-process to avoid an HTTP
	// round-trip that breaks when the external port differs from the internal one
	// (e.g. inside a container behind a port mapping). Works for any local path.
	if strings.HasPrefix(srcURL, "/") {
		req := r.Clone(r.Context())
		req.URL, _ = url.Parse(srcURL)
		req.RequestURI = srcURL
		rec := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			http.Error(w, rec.Body.String(), http.StatusBadGateway)
			return
		}
		renderPlot(w, rec.Body, opts)
		return
	}

	if _, err := url.ParseRequestURI(srcURL); err != nil {
		http.Error(w, fmt.Sprintf("invalid source URL: %v", err), http.StatusBadRequest)
		return
	}

	resp, err := http.Get(srcURL) //nolint:noctx
	if err != nil {
		http.Error(w, fmt.Sprintf("fetch source: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		http.Error(w,
			fmt.Sprintf("source returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
			http.StatusBadGateway)
		return
	}

	renderPlot(w, resp.Body, opts)
}

// buildIncplotArgs constructs the incplot CLI argument list from RenderOptions.
// Centralises flag-building previously duplicated between generateHTML and svgHandler.
func buildIncplotArgs(opts RenderOptions) []string {
	args := []string{"-w", opts.Width, "--color-scheme", opts.Theme}

	switch opts.Format {
	case "svg", "text":
		// ANSI/plain output — no --html, no --font (font is only meaningful for
		// HTML output; passing it in terminal mode triggers a warning on stdout).
	default: // "html"
		args = append(args, "--font", opts.Font)
		if opts.Canvas {
			args = append(args, "--html-canvas")
		} else {
			args = append(args, "--html")
		}
		// --override-background is only meaningful for HTML output.
		if bg, ok := themeBG[opts.Theme]; ok {
			args = append(args,
				"--override-background",
				fmt.Sprintf("%d", bg[0]),
				fmt.Sprintf("%d", bg[1]),
				fmt.Sprintf("%d", bg[2]),
			)
		}
	}

	switch opts.PlotType {
	case "line":    args = append(args, "--line")
	case "scatter": args = append(args, "--scatter")
	case "barV":    args = append(args, "--barV")
	case "barHS":   args = append(args, "--barHS")
	case "barHM":   args = append(args, "--barHM")
	case "barVM":   args = append(args, "--barVM")
	}

	return args
}

// renderPlot runs incplot with src as stdin and writes the result to w.
func renderPlot(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	switch opts.PlotType {
	case "hist", "box", "barH":
		renderTextChart(w, src, opts)
		return
	case "heatmap", "treemap", "sparkline":
		renderGotuiPlot(w, src, opts)
		return
	}
	switch opts.Format {
	case "svg":
		renderPlotSVG(w, src, opts)
	case "svg2":
		renderPlotSVGFromHTML(w, src, opts)
	case "text":
		renderPlotText(w, src, opts)
	default:
		renderPlotHTML(w, src, opts)
	}
}

func renderPlotHTML(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	cmd := newCommand(incplotBinary, buildIncplotArgs(opts)...)
	cmd.Stdin = src

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		// Return 200 so HTMX swaps the error fragment into the DOM.
		fmt.Fprintf(w, `<pre class="error">%s</pre>`, html.EscapeString(msg))
		return
	}

	result := stripIncplotDiagnostics(out.String())
	if strings.HasPrefix(result, "Error encountered") {
		fmt.Fprintf(w, `<pre class="error">%s</pre>`, html.EscapeString(strings.TrimSpace(result)))
		return
	}

	if opts.Fragment {
		result = bodyFragment(result)
	} else {
		const marginReset = `<style>html,body{margin:0;padding:0;overflow:hidden}</style>`
		result = strings.Replace(result, "</head>", marginReset+"\n</head>", 1)
		result = strings.Replace(result, "</body>", resizeScript+"\n</body>", 1)
	}
	_, _ = io.WriteString(w, result)
}

func renderPlotSVG(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	cmd := newCommand(incplotBinary, buildIncplotArgs(opts)...)
	cmd.Stdin = src

	var ansiOut, errBuf bytes.Buffer
	cmd.Stdout = &ansiOut
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		http.Error(w, fmt.Sprintf("incplot: %v: %s", err, errBuf.String()), http.StatusInternalServerError)
		return
	}

	clean := strings.NewReader(stripIncplotDiagnostics(ansiOut.String()))

	svgOpts := ansitosvg.DefaultOptions
	svgOpts.FontName = opts.Font
	svgOpts.Transparent = true
	svgOpts.LineHeight = svgLineHeight(opts.Font)
	charWidthPx := svgCharWidthPx(opts.Font, svgOpts.FontSize)
	svgOpts.CharBoxSize.X = charWidthPx
	if charWidthPx > 0 {
		svgOpts.CharBoxSize.Y = int(math.Round(float64(svgOpts.LineHeight) * float64(svgOpts.FontSize)))
		svgOpts.MarginSize.X = float32(charWidthPx) * 0.5
	} else {
		svgOpts.MarginSize.X = 0.5
	}

	var buf bytes.Buffer
	if err := ansitosvg.Convert(clean, &buf, svgOpts); err != nil {
		http.Error(w, fmt.Sprintf("svg convert: %v", err), http.StatusInternalServerError)
		return
	}

	result := injectSVGBackground(buf.String(), opts.Theme)
	result = injectSVGFonts(result)
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = fmt.Fprint(w, result)
}

// renderPlotSVGFromHTML runs incplot in HTML mode, then transforms the HTML
// output to SVG using htmlToSVG rather than ansitosvg.  This avoids the need
// for system font file access: the WOFF2 already embedded in the HTML output
// is reused verbatim, and character dimensions are derived from incplot's known
// monospace advance ratio (0.5 em) without querying sfnt metrics.
func renderPlotSVGFromHTML(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	htmlOpts := opts
	htmlOpts.Format = "html"
	htmlOpts.Canvas = false // canvas mode produces a <canvas> element, not a <pre>

	cmd := newCommand(incplotBinary, buildIncplotArgs(htmlOpts)...)
	cmd.Stdin = src

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		http.Error(w, fmt.Sprintf("incplot: %v: %s", err, msg), http.StatusInternalServerError)
		return
	}

	charWidthPx := float64(svgCharWidthPx(opts.Font, h2sFontSize))
	lineHeightPx := float64(int(math.Round(float64(svgLineHeight(opts.Font)) * h2sFontSize)))
	svgContent, err := htmlToSVG(out.String(), h2sFontSize, charWidthPx, lineHeightPx)
	if err != nil {
		http.Error(w, fmt.Sprintf("html2svg: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = io.WriteString(w, svgContent)
}

func renderPlotText(w http.ResponseWriter, src io.Reader, opts RenderOptions) {
	cmd := newCommand(incplotBinary, buildIncplotArgs(opts)...)
	cmd.Stdin = src

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		http.Error(w, strings.TrimSpace(errBuf.String()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, stripIncplotDiagnostics(out.String()))
}

// resizeScript is injected into full (non-fragment) HTML pages so that
// the plot scales to fit the iframe width and notifies the parent of its height.
const resizeScript = `<script>
(function(){
  function fit() {
    var pre = document.querySelector(".term-output");
    if (pre) {
      var vw = document.documentElement.clientWidth;
      var pw = pre.scrollWidth;
      if (pw > vw) { pre.style.zoom = vw / pw; }
    }
    window.parent.postMessage({incplotHeight: document.body.scrollHeight}, "*");
  }
  if (document.fonts && document.fonts.ready) {
    fit();
    document.fonts.ready.then(fit);
  } else {
    window.addEventListener("load", fit);
  }
})();
</script>`

// bodyFragment extracts the <style> block and <body> content for HTMX insertion.
// The <body> tag may carry attributes (e.g. <body margin:0; style="…">), so we
// search case-insensitively for "<body" and skip to the ">" that closes it.
func bodyFragment(fullHTML string) string {
	style := ""
	si := strings.Index(fullHTML, "<style")
	se := strings.Index(fullHTML, "</style>")
	if si >= 0 && se > si {
		style = fullHTML[si:se+len("</style>")] + "\n"
	}
	lower := strings.ToLower(fullHTML)
	bi := strings.Index(lower, "<body")
	if bi < 0 {
		return style
	}
	tagEnd := strings.Index(fullHTML[bi:], ">")
	if tagEnd < 0 {
		return style
	}
	bodyStart := bi + tagEnd + 1
	ej := strings.LastIndex(lower, "</body>")
	if ej <= bodyStart {
		return style + fullHTML[bodyStart:]
	}
	return style + fullHTML[bodyStart:ej]
}

func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return s
	}
	i += len(open)
	j := strings.LastIndex(s, close)
	if j <= i {
		return s
	}
	return s[i:j]
}

// incplotDiagnosticPrefixes are the line-start patterns incplot uses for
// diagnostic output written to stdout alongside the plot.  They appear after
// the actual plot content and must be stripped before the output is rendered.
var incplotDiagnosticPrefixes = []string{
	"Warning:",
	"Incplot ",
	"Error encountered",
	"Using fallback",
	"A font was specified",
	"Without a proper fallback",
	"No fallback font",
	"Internal error",
}

// stripIncplotDiagnostics removes incplot diagnostic/warning lines that incplot
// writes to stdout after the plot content.  It finds the first line matching a
// known diagnostic prefix and truncates the string there, then restores a
// single trailing newline so ANSI and HTML consumers see a clean stream.
func stripIncplotDiagnostics(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		for _, p := range incplotDiagnosticPrefixes {
			if strings.HasPrefix(line, p) {
				return strings.TrimRight(strings.Join(lines[:i], "\n"), "\n") + "\n"
			}
		}
	}
	return s
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
