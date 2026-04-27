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
      // One data point — draw a flat horizontal line at mid-height with a
      // terminal dot, so the row looks like a sparkline even with just one
      // value. Keeps visual parity with multi-point rows; previously this
      // case rendered as plain text and looked like a broken layout next
      // to populated rows on the same card.
      var v0 = values[0];
      var single = opts.displayValue != null ? opts.displayValue
        : (opts.formatLatest ? opts.formatLatest(v0.y) : String(v0.y));
      var midY = height / 2;
      var x1 = padX;
      var x2 = width - padX;
      return '<div class="run-stats-spark-row">' +
        '<span class="run-stats-spark-label">' + esc(label) + '</span>' +
        '<svg class="run-stats-spark" viewBox="0 0 ' + width + ' ' + height + '" preserveAspectRatio="none" width="' + width + '" height="' + height + '">' +
          '<line x1="' + x1.toFixed(1) + '" y1="' + midY.toFixed(1) + '" x2="' + x2.toFixed(1) + '" y2="' + midY.toFixed(1) + '" stroke="' + color + '" stroke-width="1.5" />' +
          '<circle cx="' + x2.toFixed(1) + '" cy="' + midY.toFixed(1) + '" r="2.5" fill="' + color + '" data-run="' + v0.run + '"><title>run #' + v0.run + ': ' + v0.tooltip + '</title></circle>' +
        '</svg>' +
        '<span class="run-stats-spark-latest" title="latest run">' + esc(single) + '</span>' +
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
    var latestStr = opts.displayValue != null ? opts.displayValue
      : (opts.formatLatest ? opts.formatLatest(latest.y) : String(latest.y));
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

    fetchJSON('/api/tasks/' + encodeURIComponent(taskID) + '/runs').then(function (runsRaw) {
      var runs = runsRaw || [];
      if (runs.length === 0) {
        containerEl.innerHTML = '<div class="run-stats-card run-stats-empty-state">No runs yet. Stats will appear after the first run completes.</div>';
        return;
      }
      // Fetch host_samples for ALL runs in parallel so the sparklines can
      // walk every periodic tick chronologically across the task's history,
      // not just the latest run. Each call is cached server-side at max-age=5.
      // listRuns returns newest-first per repository.go.
      return Promise.all(runs.map(function (r) {
        return fetchJSON('/api/task_runs/' + r.id + '/host_samples')
          .catch(function () { return []; });
      })).then(function (samplesPerRun) {
        return { runs: runs, latest: runs[0], samplesPerRun: samplesPerRun };
      });
    }).then(function (ctx) {
      if (!ctx) return;
      var runs = ctx.runs;
      var latest = ctx.latest;

      // Cumulative + within-run periodic timeseries.
      //
      // Walk runs oldest → newest. For each run, walk its host_samples
      // (one tick every ~5s recorded by the host sampler — same source
      // CPU/RSS use). At each tick, compute the cross-run cumulative
      // value: priorRunsTotal + thisRunCounter. cost/turns/actions
      // monotonically increase across the whole task lifecycle.
      //
      // CPU% / RSS MB stay physical (within-run absolute) — a cumulative
      // CPU has no meaning. We DO concatenate cpu/rss samples across
      // runs so the chart shows full history, not just the last run.
      //
      // Legacy-run fallback: runs without host_samples (older sampler,
      // or runs where the sampler crashed) get a single synthesized
      // endpoint at run completion using the run row's recorded counters
      // — keeps the line connected and the right-side number honest.
      var sortedRuns = runs.slice().reverse();        // oldest first
      var sortedSamples = ctx.samplesPerRun.slice().reverse();

      var costPts = [], turnPts = [], actionPts = [], cpuPts = [], rssPts = [];
      var priorCost = 0, priorTurns = 0, priorActions = 0;

      // Per-tick DELTAS (rate of change) — visually matches CPU/RSS
      // (oscillating), not monotonic-cumulative which renders as a
      // smooth slope. Right-side numbers are CUMULATIVE totals across
      // all runs, displayed via displayValue override on the sparkline
      // so the chart shape doesn't dictate the headline figure.
      // cpu/rss stay absolute (physical metrics).
      sortedRuns.forEach(function (r, ri) {
        var samples = sortedSamples[ri] || [];
        if (samples.length === 0) {
          // Legacy run with no host_samples — synthesize endpoint with
          // final counters so the line still has a delta to render.
          samples = [{
            ts: r.completed_at || r.started_at || '',
            cost_usd: r.cost_usd || 0,
            turns: r.turns || 0,
            actions: r.actions || 0,
            cpu_pct: 0,
            rss_mb: 0,
          }];
        }
        var prevCost = 0, prevTurns = 0, prevActions = 0;
        samples.forEach(function (sm) {
          var rn = r.run_number;
          var t = (sm.ts || '').slice(11, 19);
          var dC = Math.max(0, (sm.cost_usd || 0) - prevCost);
          var dT = Math.max(0, (sm.turns || 0) - prevTurns);
          var dA = Math.max(0, (sm.actions || 0) - prevActions);
          prevCost = sm.cost_usd || 0;
          prevTurns = sm.turns || 0;
          prevActions = sm.actions || 0;
          costPts.push({ run: rn, y: dC,
            tooltip: '+' + fmtCost(dC) + ' tick (run #' + rn + ' at ' + t + ')' });
          turnPts.push({ run: rn, y: dT,
            tooltip: '+' + dT + ' turns (run #' + rn + ' at ' + t + ')' });
          actionPts.push({ run: rn, y: dA,
            tooltip: '+' + dA + ' actions (run #' + rn + ' at ' + t + ')' });
          cpuPts.push({ run: rn, y: sm.cpu_pct || 0,
            tooltip: (sm.cpu_pct || 0).toFixed(1) + '% (run #' + rn + ' at ' + t + ')' });
          rssPts.push({ run: rn, y: sm.rss_mb || 0,
            tooltip: Math.round(sm.rss_mb || 0) + ' MB (run #' + rn + ' at ' + t + ')' });
        });
        priorCost += r.cost_usd || 0;
        priorTurns += r.turns || 0;
        priorActions += r.actions || 0;
      });

      // Subsample to 20 points per series — matches the original visual
      // density before the periodic-cumulative refactor. subsample
      // preserves first + last so the run-end synthetic points always
      // make it into the rendered chart, keeping the right-side number
      // = footer cumulative.
      var cumCost = priorCost, cumTurns = priorTurns, cumActions = priorActions;
      costPts = subsample(costPts, 20);
      turnPts = subsample(turnPts, 20);
      actionPts = subsample(actionPts, 20);
      cpuPts = subsample(cpuPts, 20);
      rssPts = subsample(rssPts, 20);

      var when = latest.completed_at ? timeAgo(latest.completed_at) : 'in progress';
      var dur = fmtDuration(latest.started_at, latest.completed_at);
      var modelLine = latest.model
        ? esc(latest.model) + ' · pricing ' + esc(latest.pricing_version || '—')
        : '<span class="run-stats-empty">model not recorded — re-run to populate</span>';

      // Cumulative footer totals across ALL runs. Peaks are MAX (not sum) —
      // "peak" by definition is the highest sample seen.
      var totalLines = 0, peakCpu = 0, peakRss = 0;
      runs.forEach(function (r) {
        totalLines += r.lines || 0;
        if ((r.peak_cpu_pct || 0) > peakCpu) peakCpu = r.peak_cpu_pct;
        if ((r.peak_rss_mb || 0) > peakRss) peakRss = r.peak_rss_mb;
      });

      containerEl.innerHTML =
        '<div class="run-stats-card">' +
          '<div class="run-stats-header">' +
            '<span class="run-stats-title">Run Stats — ' + runs.length + ' run' + (runs.length === 1 ? '' : 's') + ' (cumulative)</span>' +
            '<span class="run-stats-when">latest run #' + latest.run_number + ' · ' + esc(when) + ' · ' + esc(dur) + '</span>' +
          '</div>' +
          '<div class="run-stats-tokens-row">' +
            '<div class="run-stats-tokens-label">Token mix (run #' + latest.run_number + ')</div>' +
            renderTokenBar(latest) +
          '</div>' +
          '<div class="run-stats-detail-row">' +
            renderCacheRing(latest) +
            '<div class="run-stats-sparks">' +
              renderSparkline('cost', costPts, { color: 'var(--green)', displayValue: fmtCost(cumCost) }) +
              renderSparkline('turns', turnPts, { color: 'var(--accent)', displayValue: String(cumTurns) }) +
              renderSparkline('actions', actionPts, { color: 'var(--yellow)', displayValue: String(cumActions) }) +
              // Host metrics stay within-run (last completed run's samples)
              // — physical metrics like CPU% have no cumulative interpretation.
              // Orange CPU%, purple RSS MB. Empty arrays render as a muted
              // "no samples" placeholder so the card layout stays stable.
              renderSparkline('cpu%', cpuPts, { color: '#f59e0b', formatLatest: function (v) { return v.toFixed(0) + '%'; } }) +
              renderSparkline('rss mb', rssPts, { color: '#a855f7', formatLatest: function (v) { return Math.round(v) + ''; } }) +
            '</div>' +
          '</div>' +
          '<div class="run-stats-footer">' +
            '<span>Turns ' + cumTurns + '</span>' +
            '<span>Actions ' + cumActions + '</span>' +
            '<span>Lines ' + totalLines + '</span>' +
            (peakCpu > 0 ? '<span>Peak CPU ' + peakCpu.toFixed(0) + '%</span>' : '') +
            (peakRss > 0 ? '<span>Peak RSS ' + Math.round(peakRss) + ' MB</span>' : '') +
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
