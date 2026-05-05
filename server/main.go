package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	defaultWidth = "80"
	defaultFont  = "Adwaita Mono"
	defaultTheme = "solarized_light"
	defaultPort  = "8080"
	basePath     = "/incplot"
)

// themeBG maps colour scheme name to background RGB for --override-background.
var themeBG = map[string][3]int{
	"solarized_light": {253, 246, 227},
	"one_half_light":  {250, 250, 250},
	"tango_light":     {255, 255, 255},
	"dimidium":        {20, 20, 20},
	"one_half_dark":   {40, 44, 52},
	"solarized_dark":  {0, 43, 54},
	"dark_plus":       {30, 30, 30},
	"campbell":        {12, 12, 12},
	"monochrome":      {13, 13, 13},
}

var incplotBinary = func() string {
	if v := os.Getenv("INCPLOT_BIN"); v != "" {
		return v
	}
	return "/usr/local/bin/incplot"
}()

var envDefaultFont = func() string {
	if v := os.Getenv("INCPLOT_DEFAULT_FONT"); v != "" {
		return v
	}
	return defaultFont
}()

var (
	monoFontsOnce sync.Once
	monoFonts     []string
)

// availableMonoFonts returns the list of monospace font family names available
// on the system via fc-list, with the default font first.
func availableMonoFonts() []string {
	monoFontsOnce.Do(func() {
		out, err := exec.Command("fc-list", ":spacing=mono", "family").Output()
		if err != nil {
			monoFonts = []string{envDefaultFont}
			return
		}
		seen := map[string]bool{}
		var fonts []string
		skip := []string{"Nerd Font", "Nerd Font Mono", " NF", " NFM", "SignWrit", "MathJax", "Color Emoji"}
		for _, line := range strings.Split(string(out), "\n") {
			// fc-list may return "Family,Alias" — take the first token.
			name := strings.TrimSpace(strings.SplitN(line, ",", 2)[0])
			if name == "" || seen[name] {
				continue
			}
			excluded := false
			for _, s := range skip {
				if strings.Contains(name, s) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
			seen[name] = true
			fonts = append(fonts, name)
		}
		// Sort so default appears first, then alphabetical.
		sorted := []string{envDefaultFont}
		for _, f := range fonts {
			if f != envDefaultFont {
				sorted = append(sorted, f)
			}
		}
		monoFonts = sorted
	})
	return monoFonts
}

// fontOptionsHTML builds <option> elements for the font <select>.
func fontOptionsHTML() string {
	var sb strings.Builder
	for _, f := range availableMonoFonts() {
		sb.WriteString(`<option value="`)
		sb.WriteString(f)
		sb.WriteString(`">`)
		sb.WriteString(f)
		sb.WriteString("</option>\n")
	}
	return sb.String()
}

// embedJS is served at /incplot/embed.js. Include it once on any host page
// and all incplot <iframe> elements will auto-size to their content height.
const embedJS = `(function(){
  window.addEventListener("message", function(e) {
    if (!e.data || !e.data.incplotHeight) return;
    var iframes = document.querySelectorAll("iframe");
    for (var i = 0; i < iframes.length; i++) {
      if (iframes[i].contentWindow === e.source) {
        iframes[i].style.height = e.data.incplotHeight + "px";
      }
    }
  });
})();`

func embedJSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = io.WriteString(w, embedJS)
}

func uiHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page := strings.ReplaceAll(uiHTML, "/*FONT_CSS*/", inlineFontCSS())
	page = strings.ReplaceAll(page, "/*DEFAULT_FONT*/", envDefaultFont)
	page = strings.ReplaceAll(page, "/*FONT_OPTIONS*/", fontOptionsHTML())
	page = strings.ReplaceAll(page, "/*BASE_PATH*/", basePath)
	_, _ = io.WriteString(w, page)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	http.HandleFunc(basePath, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, basePath+"/ui", http.StatusMovedPermanently)
	})
	http.HandleFunc(basePath+"/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, basePath+"/ui", http.StatusMovedPermanently)
	})
	http.HandleFunc(basePath+"/ui", uiHandler)
	http.HandleFunc(basePath+"/embed.js", embedJSHandler)

	// Source layer — CORS-enabled NDJSON data endpoints.
	http.HandleFunc(basePath+"/sources", withCORS(sourcesHandler))
	http.HandleFunc(basePath+"/source/", withCORS(sourceHandler))
	http.HandleFunc(basePath+"/data", withCORS(dataHandler))

	// Render layer — source URL → incplot → output format.
	http.HandleFunc(basePath+"/plot", plotHandler)
	http.HandleFunc(basePath+"/render", renderHandler)

	mcpH := newMCPHandler()
	http.Handle(basePath+"/mcp", mcpH)
	http.Handle(basePath+"/mcp/", mcpH)

	log.Printf("incplot-server listening on :%s (base path: %s)  MCP at %s/mcp/", port, basePath, basePath)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
