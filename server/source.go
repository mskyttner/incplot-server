package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

//go:embed data/*
var dataFS embed.FS

// SourceMacro is a named data source.
// Built-in sources have a DataFile (embedded in data/) and a SelectSQL that
// references /dev/stdin via read_csv or read_ndjson.
// Dynamically registered sources have no DataFile; SelectSQL is executed directly.
type SourceMacro struct {
	Name        string
	Label       string
	Description string
	DataFile    string // embedded filename under data/ (empty for dynamic sources)
	SelectSQL   string // SQL executed by duckdb; built-ins use /dev/stdin as data path
	DefaultType string // suggested plot type for the UI (e.g. "line")
}

// sourceRegistry holds built-in and dynamically registered named sources.
var sourceRegistry sync.Map // string → SourceMacro

var builtinSources = []SourceMacro{
	{
		Name:        "german_economy",
		Label:       "German Economy 1991–2023",
		Description: "GDP contribution by sector, Germany, 1991–2023",
		DataFile:    "german_economy.tsv",
		SelectSQL:   `SELECT * FROM read_csv('/dev/stdin', delim:=chr(9), auto_detect:=true)`,
		DefaultType: "line",
	},
	{
		Name:        "euro_economies",
		Label:       "European Economies",
		Description: "GDP data for European countries",
		DataFile:    "Euro_economies.tsv",
		SelectSQL:   `SELECT * FROM read_csv('/dev/stdin', delim:=chr(9), auto_detect:=true)`,
		DefaultType: "line",
	},
	{
		Name:        "iris",
		Label:       "Iris Dataset",
		Description: "Fisher's iris dataset — sepal/petal measurements by species",
		DataFile:    "iris_data_small.ndjson",
		SelectSQL:   `SELECT * FROM read_ndjson('/dev/stdin', auto_detect:=true)`,
		DefaultType: "scatter",
	},
	{
		Name:        "wine_quality",
		Label:       "Wine Quality",
		Description: "Physicochemical properties and quality scores of wines",
		DataFile:    "wine_quality_data_small.csv",
		SelectSQL:   `SELECT * FROM read_csv('/dev/stdin', auto_detect:=true)`,
		DefaultType: "scatter",
	},
}

func init() {
	for _, s := range builtinSources {
		sourceRegistry.Store(s.Name, s)
	}
}

// slugRe matches valid source names: lowercase alphanumeric, hyphens, underscores.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// withCORS wraps a handler with Access-Control-Allow-Origin: * headers.
// Applied to all source/data endpoints so they can be fetched cross-origin.
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// sourcesHandler serves GET /incplot/sources (JSON listing) and
// POST /incplot/sources (register a dynamic named source).
func sourcesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listSources(w, r)
	case http.MethodPost:
		registerSource(w, r)
	default:
		http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
	}
}

type sourceInfo struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	URL         string `json:"url"`
	DefaultType string `json:"default_type,omitempty"`
	Builtin     bool   `json:"builtin"`
}

func listSources(w http.ResponseWriter, r *http.Request) {
	var list []sourceInfo
	// Built-ins first, in declared order.
	for _, s := range builtinSources {
		list = append(list, sourceInfo{
			Name:        s.Name,
			Label:       s.Label,
			Description: s.Description,
			URL:         basePath + "/source/" + s.Name,
			DefaultType: s.DefaultType,
			Builtin:     true,
		})
	}
	// Dynamic sources registered at runtime.
	sourceRegistry.Range(func(k, v any) bool {
		s := v.(SourceMacro)
		if s.DataFile == "" { // dynamic = no embedded file
			list = append(list, sourceInfo{
				Name:        s.Name,
				Label:       s.Label,
				Description: s.Description,
				URL:         basePath + "/source/" + s.Name,
				DefaultType: s.DefaultType,
				Builtin:     false,
			})
		}
		return true
	})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"sources": list})
}

type registerRequest struct {
	SQL         string `json:"sql"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	DefaultType string `json:"default_type"`
}

func registerSource(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	req.SQL = strings.TrimSpace(req.SQL)
	req.Name = strings.TrimSpace(strings.ToLower(req.Name))
	if req.SQL == "" {
		http.Error(w, `"sql" is required`, http.StatusBadRequest)
		return
	}
	if !slugRe.MatchString(req.Name) {
		http.Error(w, `"name" must be lowercase alphanumeric/underscore/hyphen, 1–64 chars`, http.StatusBadRequest)
		return
	}
	// Reject attempts to overwrite built-in sources.
	for _, b := range builtinSources {
		if b.Name == req.Name {
			http.Error(w, fmt.Sprintf("name %q is reserved for a built-in source", req.Name), http.StatusConflict)
			return
		}
	}
	// Validate SQL by running a LIMIT 1 probe.
	probeSQL := fmt.Sprintf("SELECT * FROM (%s) LIMIT 1", req.SQL)
	cmd := duckdbCommand(probeSQL)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		http.Error(w, fmt.Sprintf("SQL validation failed: %s", msg), http.StatusUnprocessableEntity)
		return
	}
	label := req.Label
	if label == "" {
		label = req.Name
	}
	s := SourceMacro{
		Name:        req.Name,
		Label:       label,
		Description: req.Description,
		SelectSQL:   req.SQL,
		DefaultType: req.DefaultType,
	}
	sourceRegistry.Store(req.Name, s)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"name": req.Name,
		"url":  basePath + "/source/" + req.Name,
	})
}

// sourceHandler serves GET /incplot/source/{name} — streams NDJSON for a named source.
func sourceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, basePath+"/source/")
	name = strings.Trim(name, "/")
	if name == "" {
		http.Error(w, "source name required", http.StatusBadRequest)
		return
	}
	v, ok := sourceRegistry.Load(name)
	if !ok {
		http.Error(w, fmt.Sprintf("unknown source %q", name), http.StatusNotFound)
		return
	}
	src := v.(SourceMacro)
	streamSource(w, src)
}

func streamSource(w http.ResponseWriter, src SourceMacro) {
	cmd := duckdbCommand(src.SelectSQL)

	if src.DataFile != "" {
		data, err := dataFS.ReadFile("data/" + src.DataFile)
		if err != nil {
			http.Error(w, fmt.Sprintf("data file: %v", err), http.StatusInternalServerError)
			return
		}
		cmd.Stdin = bytes.NewReader(data)
	}

	var errBuf strings.Builder
	cmd.Stderr = &errBuf

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		http.Error(w, fmt.Sprintf("duckdb: %s", msg), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}

// dataHandler serves GET /incplot/data?sql=... — ad-hoc SQL as NDJSON.
func dataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	sql := strings.TrimSpace(r.URL.Query().Get("sql"))
	if sql == "" {
		http.Error(w, "sql query parameter is required", http.StatusBadRequest)
		return
	}
	cmd := duckdbCommand(sql)
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		http.Error(w, fmt.Sprintf("duckdb: %s", msg), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}
