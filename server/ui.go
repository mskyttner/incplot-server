package main

const uiHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8"/>
<title>incplot</title>
<style>
:root {
  --base3:  #fdf6e3; --base2:  #eee8d5; --base1:  #93a1a1;
  --base0:  #839496; --base00: #657b83; --base01: #586e75;
  --blue:   #268bd2; --cyan:   #2aa198; --red:    #dc322f;
  --green:  #859900;
}
/*FONT_CSS*/
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  background: var(--base3); color: var(--base01);
  font-family: "Adwaita Mono", monospace; font-size: 13px;
  height: 100vh; display: flex; flex-direction: column; overflow: hidden;
}
header {
  padding: 0.8rem 2rem;
  border-bottom: 1px solid var(--base2);
  color: var(--base1); font-size: 0.75rem;
  letter-spacing: 0.18em; text-transform: uppercase;
  flex: 0 0 auto;
}
.form-area { padding: 1.25rem 2rem 0; flex: 0 0 auto; }
.row { margin-bottom: 0.6rem; }
textarea {
  display: block; width: 100%; height: 8rem;
  padding: 0.5rem 0.6rem;
  font-family: inherit; font-size: inherit;
  background: var(--base2); color: var(--base01);
  border: 1px solid var(--base1); resize: vertical;
}
textarea:focus { outline: 1px solid var(--blue); }
.controls {
  display: flex; flex-wrap: wrap; gap: 1.2rem;
  align-items: flex-end; margin-top: 0.75rem;
}
label {
  display: flex; flex-direction: column; gap: 0.25rem;
  font-size: 0.7rem; color: var(--base1);
  letter-spacing: 0.12em; text-transform: uppercase;
}
select, input[type=text], input[type=number] {
  padding: 0.3rem 0.5rem;
  background: var(--base2); color: var(--base01);
  border: 1px solid var(--base1);
  font-family: inherit; font-size: 0.9rem;
}
select:focus, input:focus { outline: 1px solid var(--blue); }
.checkbox-label {
  flex-direction: row; align-items: center; gap: 0.4rem;
  padding-bottom: 0.2rem;
}
button {
  padding: 0.35rem 1.4rem;
  background: var(--blue); color: var(--base3);
  border: none; font-family: inherit; font-size: 0.9rem;
  cursor: pointer; letter-spacing: 0.05em;
}
button:hover { background: var(--cyan); }
button.secondary {
  background: var(--base2); color: var(--base01);
  border: 1px solid var(--base1); font-size: 0.8rem;
  padding: 0.25rem 0.8rem;
}
button.secondary:hover { background: var(--base3); }
button.save-btn {
  background: var(--green); font-size: 0.8rem;
  padding: 0.25rem 0.8rem;
}
button.save-btn:hover { background: var(--cyan); }
pre.error { padding: 1rem 2rem; color: var(--red); }

/* ── Scroll area ─────────────────────────────────────────── */
#scroll-area {
  flex: 1; overflow-y: auto; min-height: 0;
}

/* ── Info bar ────────────────────────────────────────────── */
#info-bar {
  display: none;
  padding: 0.5rem 2rem 0.8rem;
  font-size: 0.7rem; letter-spacing: 0.1em; text-transform: uppercase;
}
.source-row {
  display: flex; gap: 0.5rem; align-items: baseline; margin-top: 0.2rem;
}
.source-label { flex: 0 0 auto; color: var(--base1); }
.source-link {
  color: var(--blue); text-decoration: none;
  font-size: 0.75rem; letter-spacing: 0; text-transform: none;
  word-break: break-all;
}
.source-link:hover { text-decoration: underline; }
.section-heading {
  display: flex; align-items: center; gap: 0.5rem;
  margin-top: 0.6rem; padding-top: 0.4rem;
  border-top: 1px solid var(--base2);
}
.section-heading a.section-title {
  color: var(--base01); text-decoration: none; flex: 0 0 auto;
}
.section-heading a.section-title:hover { color: var(--blue); text-decoration: underline; }
.section-rule { flex: 1; height: 1px; background: var(--base2); }
.section-btns { display: flex; gap: 0.4rem; }

/* ── Embedded previews ───────────────────────────────────── */
.embed-preview { margin-top: 0.4rem; }
.embed-preview iframe {
  display: block; width: 100%; border: none; min-height: 2rem;
}
.embed-preview img { display: block; max-width: 100%; }
.ansi-link {
  display: inline-block; margin-top: 0.4rem;
  color: var(--blue); text-decoration: none;
  font-size: 0.75rem; letter-spacing: 0; text-transform: none;
  word-break: break-all;
}
.ansi-link:hover { text-decoration: underline; }

/* ── Save source ─────────────────────────────────────────── */
#save-source-row { display: none; gap: 0.5rem; align-items: center; margin-top: 0.35rem; }
#save-source-row input { width: 14rem; }
</style>
<script>
var BASE = '/*BASE_PATH*/';
var sources = [];

// ── Source management ───────────────────────────────────────────────────────

function loadSources() {
  fetch(BASE + '/sources')
    .then(function(r) { return r.json(); })
    .then(function(data) {
      sources = data.sources || [];
      var sel = document.getElementById('source-sel');
      while (sel.options.length > 1) sel.remove(1);
      sources.forEach(function(s, i) {
        var opt = document.createElement('option');
        opt.value = i;
        opt.textContent = s.label + (s.builtin ? '' : ' *');
        sel.appendChild(opt);
      });
    });
}

function currentSourceURL() {
  var sel = document.getElementById('source-sel');
  if (sel.value === 'custom') {
    var sql = document.getElementById('custom-sql').value.trim();
    return sql ? BASE + '/data?sql=' + encodeURIComponent(sql) : '';
  }
  var idx = parseInt(sel.value, 10);
  return isNaN(idx) ? '' : sources[idx].url;
}

function onSourceChange() {
  var sel = document.getElementById('source-sel');
  var isCustom = sel.value === 'custom';
  document.getElementById('custom-area').style.display = isCustom ? 'block' : 'none';
  document.getElementById('save-source-row').style.display = 'none';
  if (!isCustom) {
    var idx = parseInt(sel.value, 10);
    if (!isNaN(idx) && sources[idx].default_type) {
      document.getElementById('type-sel').value = sources[idx].default_type;
    }
  }
}

// ── Render ──────────────────────────────────────────────────────────────────

function triggerRender() {
  var src = currentSourceURL();
  if (!src) return;
  var base = window.location.origin + BASE;

  var common = {
    source: src,
    width:  document.getElementById('width-in').value,
    font:   document.getElementById('font-in').value,
    theme:  document.getElementById('theme-sel').value,
    type:   document.getElementById('type-sel').value,
  };
  if (document.getElementById('canvas-cb').checked) common.canvas = '1';

  var htmlUrl = base + '/plot?' + new URLSearchParams(Object.assign({}, common, {format: 'html'})).toString();
  var svgUrl  = base + '/plot?' + new URLSearchParams(Object.assign({}, common, {format: 'svg'})).toString();
  var textUrl = base + '/plot?' + new URLSearchParams(Object.assign({}, common, {format: 'text'})).toString();

  // Update hidden inputs used by copy buttons
  document.getElementById('html-plot-url').value = htmlUrl;
  document.getElementById('svg-plot-url').value  = svgUrl;
  document.getElementById('text-plot-url').value = textUrl;

  // Update source hyperlink
  var srcEl = document.getElementById('source-url');
  srcEl.href = src;
  srcEl.textContent = src;

  // Update section title links
  document.getElementById('html-section-link').href = htmlUrl;
  document.getElementById('svg-section-link').href  = svgUrl;
  document.getElementById('ansi-section-link').href = textUrl;

  // Update embeds
  document.getElementById('html-frame').src = htmlUrl;
  document.getElementById('svg-img').src    = svgUrl;
  var al = document.getElementById('ansi-link');
  al.href = textUrl;
  al.textContent = textUrl;

  // Show info bar and save-source option
  document.getElementById('info-bar').style.display = 'block';
  if (document.getElementById('source-sel').value === 'custom') {
    document.getElementById('save-source-row').style.display = 'flex';
  }
}

// ── Auto-resize iframe via postMessage ───────────────────────────────────────

window.addEventListener('message', function(e) {
  if (!e.data || !e.data.incplotHeight) return;
  var iframes = document.querySelectorAll('iframe');
  for (var i = 0; i < iframes.length; i++) {
    if (iframes[i].contentWindow === e.source) {
      iframes[i].style.height = e.data.incplotHeight + 'px';
    }
  }
});

// ── Save dynamic source ──────────────────────────────────────────────────────

function saveSource() {
  var sql  = document.getElementById('custom-sql').value.trim();
  var name = document.getElementById('save-name').value.trim();
  if (!sql || !name) { alert('SQL and name are required.'); return; }
  fetch(BASE + '/sources', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({sql: sql, name: name, label: name.replace(/_/g, ' ')}),
  }).then(function(r) {
    if (!r.ok) return r.text().then(function(t) { alert('Error: ' + t); });
    return r.json().then(function() {
      loadSources();
      document.getElementById('save-source-row').style.display = 'none';
      document.getElementById('save-name').value = '';
      var btn = document.getElementById('save-btn');
      btn.textContent = 'saved!';
      setTimeout(function() { btn.textContent = 'save source'; }, 1500);
    });
  });
}

// ── Copy helpers ─────────────────────────────────────────────────────────────

function copyField(id, btn) {
  navigator.clipboard.writeText(document.getElementById(id).value).then(function() {
    var orig = btn.textContent;
    btn.textContent = 'copied!';
    setTimeout(function() { btn.textContent = orig; }, 1500);
  });
}

function copySnippet(format, btn) {
  var embed = window.location.origin + BASE + '/embed.js';
  var snip;
  if (format === 'svg') {
    var url = document.getElementById('svg-plot-url').value;
    snip = '<img src="' + url + '" alt="incplot"/>';
  } else {
    var url = document.getElementById('html-plot-url').value;
    snip = '<script src="' + embed + '"><\/script>\n<iframe src="' + url + '" scrolling="no"><\/iframe>';
  }
  navigator.clipboard.writeText(snip).then(function() {
    var orig = btn.textContent;
    btn.textContent = 'copied!';
    setTimeout(function() { btn.textContent = orig; }, 1500);
  });
}

// ── Init ─────────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', function() {
  loadSources();
  document.getElementById('custom-sql').addEventListener('input', function() {});
});
</script>
</head>
<body>
<header>incplot &mdash; source / render pipeline</header>
<div class="form-area">

  <div class="row">
    <label>Source
      <select id="source-sel" onchange="onSourceChange()" style="width:100%">
        <option value="custom">custom sql&hellip;</option>
      </select>
    </label>
  </div>

  <div id="custom-area">
    <textarea id="custom-sql" spellcheck="false"
      placeholder="SELECT n, n*n AS n_squared FROM (SELECT unnest(generate_series(1,10)) AS n)"></textarea>
  </div>

  <div class="controls">
    <label>Type
      <select id="type-sel">
        <option value="">inferred</option>
        <option value="line">line</option>
        <option value="scatter">scatter</option>
        <option value="barV">barV</option>
        <option value="barHS">barHS (stacked)</option>
        <option value="barHM">barHM (multi)</option>
        <option value="barVM">barVM (multi vert)</option>
      </select>
    </label>
    <label>Width
      <input id="width-in" type="number" value="80" min="40" max="400" style="width:5rem"/>
    </label>
    <label>Font
      <select id="font-in">
/*FONT_OPTIONS*/      </select>
    </label>
    <label>Theme
      <select id="theme-sel">
        <option value="solarized_light" selected>solarized_light</option>
        <option value="one_half_light">one_half_light</option>
        <option value="tango_light">tango_light</option>
        <option value="dimidium">dimidium</option>
        <option value="one_half_dark">one_half_dark</option>
        <option value="solarized_dark">solarized_dark</option>
        <option value="dark_plus">dark_plus</option>
        <option value="campbell">campbell</option>
        <option value="monochrome">monochrome</option>
      </select>
    </label>
    <label class="checkbox-label">
      <input id="canvas-cb" type="checkbox" value="1"/> canvas
    </label>
    <button onclick="triggerRender()">Plot</button>
  </div>

</div>

<div id="scroll-area">
<div id="info-bar">

  <!-- Hidden inputs used by copy buttons -->
  <input id="html-plot-url" type="hidden" readonly/>
  <input id="svg-plot-url"  type="hidden" readonly/>
  <input id="text-plot-url" type="hidden" readonly/>

  <div class="source-row">
    <span class="source-label">Source</span>
    <a id="source-url" class="source-link" href="#" target="_blank"></a>
  </div>

  <!-- HTML section -->
  <div class="section-heading">
    <a id="html-section-link" class="section-title" href="#" target="_blank">HTML</a>
    <span class="section-rule"></span>
    <div class="section-btns">
      <button class="secondary" onclick="copyField('html-plot-url',this)">copy url</button>
      <button class="secondary" onclick="copySnippet('html',this)">copy snippet</button>
    </div>
  </div>
  <div class="embed-preview">
    <iframe id="html-frame" scrolling="no"></iframe>
  </div>

  <!-- SVG section -->
  <div class="section-heading">
    <a id="svg-section-link" class="section-title" href="#" target="_blank">SVG</a>
    <span class="section-rule"></span>
    <div class="section-btns">
      <button class="secondary" onclick="copyField('svg-plot-url',this)">copy url</button>
      <button class="secondary" onclick="copySnippet('svg',this)">copy snippet</button>
    </div>
  </div>
  <div class="embed-preview">
    <img id="svg-img" alt=""/>
  </div>

  <!-- ANSI section -->
  <div class="section-heading">
    <a id="ansi-section-link" class="section-title" href="#" target="_blank">ANSI</a>
    <span class="section-rule"></span>
    <div class="section-btns">
      <button class="secondary" onclick="copyField('text-plot-url',this)">copy url</button>
    </div>
  </div>
  <a id="ansi-link" class="ansi-link" href="#" target="_blank"></a>

  <!-- Save custom SQL as named source -->
  <div id="save-source-row">
    <input id="save-name" type="text" placeholder="source name (slug)"/>
    <button id="save-btn" class="save-btn" onclick="saveSource()">save source</button>
  </div>

</div>
</div>
</body>
</html>`
