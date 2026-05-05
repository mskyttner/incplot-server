#!/usr/bin/env bash
# Smoke-test all chart types against a running incplot-server.
#
# Usage: ./smoketest.sh [server-base-url]
#   Default server: http://localhost:8080
#
# Requires: curl
set -euo pipefail

BASE="${1:-http://localhost:8080}/incplot"
PASS=0; FAIL=0

plot() {
    local label="$1"; shift
    printf '\n\033[1m=== %s ===\033[0m\n' "$label"
    local out http_code
    out=$(curl -sf --write-out '\n%{http_code}' "$@") || { echo "FAIL (curl error)"; ((FAIL++)); return; }
    http_code="${out##*$'\n'}"
    body="${out%$'\n'*}"
    if [[ "$http_code" == "200" ]]; then
        echo "$body" | head -8
        echo "OK ($(echo "$body" | wc -l) lines)"
        ((PASS++)) || true
    else
        echo "FAIL (HTTP $http_code): $body"
        ((FAIL++)) || true
    fi
}

# Register helper sources for types that need a specific data shape.
register() {
    curl -sf -X POST "$BASE/sources" \
      -H 'Content-Type: application/json' \
      -d "$1" > /dev/null
}

echo "Server: $BASE"
echo "Registering test sources..."

register '{
  "name":  "test-heatmap",
  "label": "Test heatmap",
  "sql":   "SELECT a, b, c FROM (VALUES (1,2,3),(4,5,6),(7,8,9),(2,4,6),(3,6,9)) t(a,b,c)",
  "default_type": "heatmap"
}'

register '{
  "name":  "test-treemap",
  "label": "Test treemap",
  "sql":   "SELECT label, val FROM (VALUES ('"'"'Go'"'"',42),('"'"'Python'"'"',89),('"'"'Rust'"'"',31),('"'"'TS'"'"',67),('"'"'C++'"'"',54)) t(label,val)",
  "default_type": "treemap"
}'

register '{
  "name":  "test-barh",
  "label": "Test barH",
  "sql":   "SELECT label, val FROM (VALUES ('"'"'Go'"'"',42),('"'"'Python'"'"',89),('"'"'Rust'"'"',31),('"'"'TS'"'"',67)) t(label,val)",
  "default_type": "barH"
}'

SRC="$BASE/source"

# incplot types
plot "line"      --get --data "source=$SRC/german_economy&type=line&format=text&width=70"     "$BASE/plot"
plot "scatter"   --get --data "source=$SRC/iris&type=scatter&format=text&width=70"            "$BASE/plot"
plot "barV"      --get --data "source=$SRC/test-treemap&type=barV&format=text&width=70"      "$BASE/plot"
plot "barHS"     --get --data "source=$SRC/test-treemap&type=barHS&format=text&width=70"     "$BASE/plot"
plot "barHM"     --get --data "source=$SRC/test-heatmap&type=barHM&format=text&width=70"     "$BASE/plot"
plot "barVM"     --get --data "source=$SRC/test-heatmap&type=barVM&format=text&width=70"     "$BASE/plot"

# gotui types
plot "heatmap"   --get --data "source=$SRC/test-heatmap&type=heatmap&format=text&width=70"   "$BASE/plot"
plot "treemap"   --get --data "source=$SRC/test-treemap&type=treemap&format=text&width=70"   "$BASE/plot"
plot "sparkline" --get --data "source=$SRC/german_economy&type=sparkline&format=text&width=70" "$BASE/plot"

# pure-Go types
plot "hist"      --get --data "source=$SRC/wine_quality&type=hist&format=text&width=70"       "$BASE/plot"
plot "box"       --get --data "source=$SRC/wine_quality&type=box&format=text&width=70"        "$BASE/plot"
plot "barH"      --get --data "source=$SRC/test-barh&type=barH&format=text&width=70"         "$BASE/plot"

printf '\n\033[1mResults: %d passed, %d failed\033[0m\n' "$PASS" "$FAIL"
[[ $FAIL -eq 0 ]]
