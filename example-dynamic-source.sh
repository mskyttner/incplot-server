#!/usr/bin/env bash
# Example: register a dynamic data source on incplot-server, then fetch plots.
#
# Usage: ./example-dynamic-source.sh [server-base-url]
#   Default server: http://localhost:8080
#
# Requires: curl, jq
set -euo pipefail

BASE="${1:-http://localhost:8080}/incplot"

# ── 1. Register a dynamic source via POST /incplot/sources ───────────────────
#
# The SQL can be anything DuckDB can execute: inline data, remote CSVs,
# parquet files, etc.  Here we generate a simple time series inline so the
# example works without any external files.

echo "Registering source 'sine_wave'..."

RESPONSE=$(curl -sf -X POST "$BASE/sources" \
  -H 'Content-Type: application/json' \
  -d '{
    "name":  "sine_wave",
    "label": "Sine wave (generated)",
    "sql":   "SELECT i AS x, round(sin(i * 0.3), 4) AS y FROM range(0, 40) t(i)",
    "default_type": "line"
  }')

SOURCE_URL=$(echo "$RESPONSE" | jq -r '.url')
echo "Registered: $SOURCE_URL"

# ── 2. Inspect the raw NDJSON the source produces ────────────────────────────

echo ""
echo "Raw NDJSON (first 4 rows):"
curl -sf "$BASE/source/sine_wave" | head -4

# ── 3. Retrieve an ANSI line plot ────────────────────────────────────────────

echo ""
echo "ANSI line plot:"
curl -sf --get \
  --data-urlencode "source=${BASE}/source/sine_wave" \
  --data-urlencode "font=Adwaita Mono" \
  --data "width=80&theme=solarized_light&type=line&format=text" \
  "$BASE/plot"

# ── 4. Verify the source now appears in the sources list ─────────────────────

echo ""
echo "Source listing:"
curl -sf "$BASE/sources" | jq '.sources[] | {name, label, builtin}'
