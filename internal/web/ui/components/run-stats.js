// OpenPraxis — Run Stats card (per-task post-run visualization)
//
// Mounted on the task detail page above the description. Surfaces the
// most recent run + trend across last N runs:
//   - Token-mix flexbox bar (input / output / cache_read / cache_creation)
//   - Cache-hit ring via CSS conic-gradient
//   - Sparklines (cost / turns / actions) via inline SVG <polyline>
//
// Tech: zero new vendor libs. SVG + flexbox + CSS conic-gradient.
// Powered by the new columns added to task_runs in PR #205.
//
// Usage:
//   OL.renderRunStats(containerEl, taskID)

(function (OL) {
  'use strict';

  var fetchJSON = OL.fetchJSON, esc = OL.esc, timeAgo = OL.timeAgo;

  // ---- helpers ---------------------------------------------------------

  function fmtTokens(n) {
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'k';
    return String(n);
  }

  function fmtCost(n) { return '$' + (n || 0).toFixed(2); }

  function fmtDuration(startedISO, completedISO) {
    if (!startedISO || !completedISO) return '—';
    var ms = new Date(completedISO) - new Date(startedISO);
    if (isNaN(ms) || ms <= 0) return '—';
    var s = Math.round(ms / 1000);
    if (s < 60) return s + 's';
    var m = Math.floor(s / 60);
    var rs = s % 60;
    return m + 'm ' + rs + 's';
  }

  // ---- token-mix bar (flexbox divs, matches cost.js precedent) -------

  function renderTokenBar(run) {
    var input = run.input_tokens || 0;
    var output = run.output_tokens || 0;
    var cacheRead = run.cache_read_tokens || 0;
    var cacheCreate = run.cache_create_tokens || 0;
    var total = input + output + cacheRead + cacheCreate;
    if (total === 0) {
      return '<div class="run-stats-empty">no token data</div>';
    }
    var segs = [
      { label: 'input',        n: input,       cls: 'in' },
      { label: 'output',       n: output,      cls: 'out' },
      { label: 'cache-read',   n: cacheRead,   cls: 'cread' },
      { label: 'cache-create', n: cacheCreate, cls: 'ccreate' },
    ];
    var bar = '<div class="run-stats-bar">';
    segs.forEach(function (s) {
      if (s.n === 0) return;
      bar += '<div class="run-stats-bar-seg run-stats-bar-' + s.cls +
             '" style="flex:' + s.n + '" title="' + s.label + ': ' + s.n.toLocaleString() + '">' +
             '<span class="run-stats-bar-label">' + s.label + ' ' + fmtTokens(s.n) + '</span>' +
             '</div>';
    });
    bar += '</div>';
    return bar;
  }

  // ---- cache-hit ring (CSS conic-gradient) ---------------------------

  function renderCacheRing(run) {
    var input = run.input_tokens || 0;
    var output = run.output_tokens || 0;
    var cacheRead = run.cache_read_tokens || 0;
    var cacheCreate = run.cache_create_tokens || 0;
    var total = input + output + cacheRead + cacheCreate;
    var pct = total > 0 ? Math.round((cacheRead / total) * 100) : 0;
    return '<div class="run-stats-ring-wrap">' +
      '<div class="run-stats-ring" style="--pct:' + pct + '" title="' + cacheRead.toLocaleString() + ' / ' + total.toLocaleString() + ' tokens served from cache">' +
        '<div class="run-stats-ring-hole">' +
          '<span class="run-stats-ring-pct">' + pct + '%</span>' +
        '</div>' +
      '</div>' +
      '<div class="run-stats-ring-label">cache hit</div>' +
      '</div>';
  }

  // ---- sparkline (inline SVG <polyline> + <circle> with hover) -------

  function renderSparkline(label, values, opts) {
    opts = opts || {};
    var width = opts.width || 200;
    var height = opts.height || 28;
    var padX = 4, padY = 4;
    var color = opts.color || 'var(--accent)';
    if (!values || values.length === 0) {
      return '<div class="run-stats-spark-row"><span class="run-stats-spark-label">' + esc(label) + '</span><span class="run-stats-empty">—</span></div>';
    }
    if (values.length === 1) {
      return '<div class="run-stats-spark-row">' +
        '<span class="run-stats-spark-label">' + esc(label) + '</span>' +
        '<span class="run-stats-spark-single" title="run #' + values[0].run + ': ' + esc(values[0].tooltip) + '">' + esc(opts.formatLatest ? opts.formatLatest(values[0].y) : String(values[0].y)) + '</span>' +
        '</div>';
    }
    var ys = values.map(function (v) { return v.y; });
    var minY = Math.min.apply(null, ys);
    var maxY = Math.max.apply(null, ys);
    if (maxY === minY) maxY = minY + 1;
    var stepX = (width - padX * 2) / (values.length - 1);
    var pts = values.map(function (v, i) {
      var x = padX + i * stepX;
      var y = height - padY - ((v.y - minY) / (maxY - minY)) * (height - padY * 2);
      return { x: x, y: y, raw: v };
    });
    var path = pts.map(function (p) { return p.x.toFixed(1) + ',' + p.y.toFixed(1); }).join(' ');
    var circles = pts.map(function (p) {
      return '<circle cx="' + p.x.toFixed(1) + '" cy="' + p.y.toFixed(1) + '" r="2.5" fill="' + color + '" data-run="' + p.raw.run + '"><title>run #' + p.raw.run + ': ' + p.raw.tooltip + '</title></circle>';
    }).join('');
    var latest = values[values.length - 1];
    var latestStr = opts.formatLatest ? opts.formatLatest(latest.y) : String(latest.y);
    return '<div class="run-stats-spark-row">' +
      '<span class="run-stats-spark-label">' + esc(label) + '</span>' +
      '<svg class="run-stats-spark" viewBox="0 0 ' + width + ' ' + height + '" preserveAspectRatio="none" width="' + width + '" height="' + height + '">' +
        '<polyline points="' + path + '" fill="none" stroke="' + color + '" stroke-width="1.5" />' +
        circles +
      '</svg>' +
      '<span class="run-stats-spark-latest" title="latest run">' + esc(latestStr) + '</span>' +
      '</div>';
  }

  // Downsample an array to at most n evenly-spaced points. Preserves
  // first + last so the sparkline endpoints reflect run start/end.
  // Small-input pass-through — no work when len <= n.
  function subsample(arr, n) {
    if (!arr || arr.length <= n) return arr || [];
    var step = (arr.length - 1) / (n - 1);
    var out = [];
    for (var i = 0; i < n; i++) {
      out.push(arr[Math.min(arr.length - 1, Math.round(i * step))]);
    }
    return out;
  }

  // ---- entry point ---------------------------------------------------

  OL.renderRunStats = function (containerEl, taskID) {
    if (!containerEl || !taskID) return;
    containerEl.innerHTML = '<div class="run-stats-loading">Loading run stats…</div>';

    Promise.all([
      fetchJSON('/api/tasks/' + encodeURIComponent(taskID) + '/runs'),
      // Host samples for the latest run — fetched after we know the run id.
      // Two separate calls are cheap (both cache at max-age=5).
    ]).then(function (r0) {
      var runs = r0[0] || [];
      if (runs.length === 0) {
        containerEl.innerHTML = '<div class="run-stats-card run-stats-empty-state">No runs yet. Stats will appear after the first run completes.</div>';
        return;
      }
      // ListRuns returns newest-first per repository.go.
      var latest = runs[0];
      return fetchJSON('/api/task_runs/' + latest.id + '/host_samples').then(function (hostSamples) {
        return { runs: runs, latest: latest, hostSamples: hostSamples || [] };
      });
    }).then(function (ctx) {
      if (!ctx) return;
      var runs = ctx.runs;
      var latest = ctx.latest;
      var hostSamples = ctx.hostSamples;
      // Build sparkline data oldest→newest, capped at last 10.
      var trend = runs.slice(0, 10).reverse();
      var costPts = trend.map(function (r) {
        return { run: r.run_number, y: r.cost_usd || 0,
          tooltip: fmtCost(r.cost_usd) + ', ' + (r.turns || 0) + ' turns, ' + (r.actions || 0) + ' actions' };
      });
      var turnPts = trend.map(function (r) {
        return { run: r.run_number, y: r.turns || 0,
          tooltip: (r.turns || 0) + ' turns, ' + fmtCost(r.cost_usd) };
      });
      var actionPts = trend.map(function (r) {
        return { run: r.run_number, y: r.actions || 0,
          tooltip: (r.actions || 0) + ' actions, ' + (r.turns || 0) + ' turns' };
      });
      // Host CPU/RSS points are a within-run time series (one per ~5s
      // sample). Different X-axis conceptually but rendered by the same
      // sparkline helper so the visual vocabulary is consistent.
      // Sub-sample to 20 points so the line stays readable even on long runs.
      var cpuPts = subsample(hostSamples, 20).map(function (s, i) {
        return { run: i, y: s.cpu_pct || 0,
          tooltip: (s.cpu_pct || 0).toFixed(1) + '% CPU at ' + (s.ts ? s.ts.slice(11, 19) : '') };
      });
      var rssPts = subsample(hostSamples, 20).map(function (s, i) {
        return { run: i, y: s.rss_mb || 0,
          tooltip: Math.round(s.rss_mb || 0) + ' MB RSS at ' + (s.ts ? s.ts.slice(11, 19) : '') };
      });

      var when = latest.completed_at ? timeAgo(latest.completed_at) : 'in progress';
      var dur = fmtDuration(latest.started_at, latest.completed_at);
      var modelLine = latest.model
        ? esc(latest.model) + ' · pricing ' + esc(latest.pricing_version || '—')
        : '<span class="run-stats-empty">model not recorded — re-run to populate</span>';

      containerEl.innerHTML =
        '<div class="run-stats-card">' +
          '<div class="run-stats-header">' +
            '<span class="run-stats-title">Run Stats — Run #' + latest.run_number + '</span>' +
            '<span class="run-stats-when">' + esc(when) + ' · ' + esc(dur) + '</span>' +
          '</div>' +
          '<div class="run-stats-tokens-row">' +
            '<div class="run-stats-tokens-label">Token mix</div>' +
            renderTokenBar(latest) +
          '</div>' +
          '<div class="run-stats-detail-row">' +
            renderCacheRing(latest) +
            '<div class="run-stats-sparks">' +
              renderSparkline('cost', costPts, { color: 'var(--green)', formatLatest: fmtCost }) +
              renderSparkline('turns', turnPts, { color: 'var(--accent)' }) +
              renderSparkline('actions', actionPts, { color: 'var(--yellow)' }) +
              // Host metrics overlay — orange CPU%, purple RSS MB. Empty
              // arrays render as a muted "no samples" placeholder so the
              // card layout stays stable during migrations / legacy runs.
              renderSparkline('cpu%', cpuPts, { color: '#f59e0b', formatLatest: function (v) { return v.toFixed(0) + '%'; } }) +
              renderSparkline('rss mb', rssPts, { color: '#a855f7', formatLatest: function (v) { return Math.round(v) + ''; } }) +
            '</div>' +
          '</div>' +
          '<div class="run-stats-footer">' +
            '<span>Turns ' + (latest.turns || 0) + '</span>' +
            '<span>Actions ' + (latest.actions || 0) + '</span>' +
            '<span>Lines ' + (latest.lines || 0) + '</span>' +
            (latest.peak_cpu_pct > 0 ? '<span>Peak CPU ' + latest.peak_cpu_pct.toFixed(0) + '%</span>' : '') +
            (latest.peak_rss_mb > 0 ? '<span>Peak RSS ' + Math.round(latest.peak_rss_mb) + ' MB</span>' : '') +
            '<span>' + modelLine + '</span>' +
          '</div>' +
        '</div>';

      // Wire click on sparkline circles → scroll the run-history section
      // into view + flash the matching row. Run history sits further down
      // the task detail panel; a brief highlight helps the eye land.
      containerEl.querySelectorAll('.run-stats-spark circle').forEach(function (c) {
        c.style.cursor = 'pointer';
        c.addEventListener('click', function () {
          var runNum = c.getAttribute('data-run');
          var row = document.querySelector('.task-run-item[data-run="' + runNum + '"]');
          if (row) {
            row.scrollIntoView({ behavior: 'smooth', block: 'center' });
            row.classList.add('scroll-highlight');
            setTimeout(function () { row.classList.remove('scroll-highlight'); }, 2000);
          }
        });
      });
    }).catch(function (e) {
      containerEl.innerHTML = '<div class="run-stats-card run-stats-empty-state">Failed to load runs: ' + esc(e.message || String(e)) + '</div>';
    });
  };
})(window.OL);
