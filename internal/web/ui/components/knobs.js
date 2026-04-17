// OpenPraxis — Execution Controls (knobs) component
//
// Shared UI for rendering the hierarchical settings catalog at product,
// manifest, or task scope. Consumes the M2 HTTP endpoints:
//   GET  /api/settings/catalog
//   GET  /api/<type>s/<id>/settings
//   PUT  /api/<type>s/<id>/settings
//   DELETE /api/<type>s/<id>/settings/<key>     (added in M3-T7)
//   GET  /api/tasks/<id>/settings/resolved       (task scope only)
//
// Integration (M3-T8/T9/T10) calls OL.renderKnobSection(containerEl, scope).

(function(OL) {
  'use strict';

  const fetchJSON = OL.fetchJSON;
  const esc = OL.esc;

  // --- catalog cache -------------------------------------------------------
  // Catalog is static per server process; fetch once and reuse across every
  // mount (products list re-render, task detail re-open, etc.).
  let _catalogCache = null;
  async function loadCatalog() {
    if (_catalogCache) return _catalogCache;
    const data = await fetchJSON('/api/settings/catalog');
    _catalogCache = (data && data.knobs) || [];
    return _catalogCache;
  }
  OL.invalidateKnobCatalogCache = function() { _catalogCache = null; };

  // --- scope-entity name cache (for provenance labels) ---------------------
  // Resolver returns source + source_id; to show "from manifest (Phase 1)"
  // we need the manifest title. Cache per scope-type/id to avoid N lookups
  // on re-render. Best-effort — missing names fall back to the short id.
  const _nameCache = { product: new Map(), manifest: new Map(), task: new Map() };
  async function lookupScopeName(scopeType, id) {
    if (!id) return '';
    const cache = _nameCache[scopeType];
    if (!cache) return id.substring(0, 8);
    if (cache.has(id)) return cache.get(id);
    let name = id.substring(0, 8);
    try {
      const data = await fetchJSON('/api/' + scopeType + 's/' + id);
      if (data && data.title) name = data.title;
      else if (data && data.name) name = data.name;
    } catch(e) { /* keep the short id fallback */ }
    cache.set(id, name);
    return name;
  }

  // --- entry point ---------------------------------------------------------
  // scope = { type: 'product'|'manifest'|'task', id: '...' }
  OL.renderKnobSection = async function(containerEl, scope) {
    if (!containerEl || !scope || !scope.type || !scope.id) return;
    containerEl.innerHTML = '<div class="knob-section-loading">Loading execution controls…</div>';
    let catalog, explicit, resolved = null;
    try {
      [catalog, explicit] = await Promise.all([
        loadCatalog(),
        fetchJSON('/api/' + scope.type + 's/' + scope.id + '/settings'),
      ]);
      if (scope.type === 'task') {
        try {
          resolved = await fetchJSON('/api/tasks/' + scope.id + '/settings/resolved');
        } catch(e) { /* resolved is non-fatal — render without inheritance */ }
      }
    } catch(e) {
      containerEl.innerHTML = '<div class="knob-section-error">Failed to load execution controls: ' + esc(e.message) + '</div>';
      return;
    }
    // Build an explicit-entries map for O(1) lookup when rendering each knob.
    const explicitByKey = new Map();
    (explicit.entries || []).forEach(e => explicitByKey.set(e.key, e));

    containerEl.innerHTML = renderSection(catalog, explicitByKey, resolved, scope);
    bindHandlers(containerEl, scope, catalog);
    // Asynchronously hydrate provenance labels for task scope (source name
    // lookups are per-scope-entity and would block the initial paint).
    if (scope.type === 'task' && resolved && resolved.resolved) {
      hydrateProvenance(containerEl, resolved.resolved);
    }
  };

  // --- rendering -----------------------------------------------------------

  function renderSection(catalog, explicitByKey, resolved, scope) {
    const rows = catalog.map(k => renderOneKnob(k, explicitByKey.get(k.key) || null, resolved, scope)).join('');
    return (
      '<div class="knob-section" data-scope-type="' + esc(scope.type) + '" data-scope-id="' + esc(scope.id) + '">' +
        '<div class="knob-section-title">Execution Controls</div>' +
        '<div class="knob-section-body">' + rows + '</div>' +
      '</div>'
    );
  }

  function renderOneKnob(knob, explicitEntry, resolved, scope) {
    // Determine the current value shown in the control. Precedence:
    //   1. explicit entry at this scope (authoritative for product/manifest/task)
    //   2. resolved value from the task inheritance walk (task scope only)
    //   3. catalog default (system tier)
    let currentValue, source = 'system', sourceID = '';
    if (explicitEntry) {
      currentValue = safeParse(explicitEntry.value);
      source = scope.type;
      sourceID = scope.id;
    } else if (resolved && resolved.resolved && resolved.resolved[knob.key]) {
      const r = resolved.resolved[knob.key];
      currentValue = r.value;
      source = r.source;
      sourceID = r.source_id;
    } else {
      currentValue = knob.default;
    }

    const control = renderControl(knob, currentValue);
    const isExplicit = !!explicitEntry;
    const resetBtn = isExplicit
      ? '<button type="button" class="knob-reset" data-knob-key="' + esc(knob.key) + '" title="Reset to inherited value">Reset</button>'
      : '';
    const provenance = renderProvenance(scope, source, sourceID, isExplicit);
    const warnings = renderWarnings(knob, currentValue);
    const saveStatus = '<span class="knob-save-status" data-knob-key="' + esc(knob.key) + '"></span>';

    return (
      '<div class="knob-row" data-knob-key="' + esc(knob.key) + '" data-knob-type="' + esc(knob.type) + '">' +
        '<div class="knob-row-main">' +
          '<label class="knob-label" title="' + esc(knob.description || '') + '">' + esc(knob.key) + '</label>' +
          control +
          saveStatus +
          resetBtn +
        '</div>' +
        '<div class="knob-row-meta">' + provenance + warnings + '</div>' +
      '</div>'
    );
  }

  function renderControl(knob, value) {
    switch (knob.type) {
      case 'int':       return renderRange(knob, value, true);
      case 'float':     return renderRange(knob, value, false);
      case 'enum':      return renderEnum(knob, value);
      case 'string':    return renderString(knob, value);
      case 'multiselect': return renderMultiselect(knob, value);
      default:
        return '<span class="knob-unsupported">unsupported type: ' + esc(knob.type) + '</span>';
    }
  }

  function renderRange(knob, value, isInt) {
    const min = knob.slider_min != null ? knob.slider_min : 0;
    const max = knob.slider_max != null ? knob.slider_max : 100;
    const step = knob.slider_step != null ? knob.slider_step : (isInt ? 1 : 0.01);
    const v = numericOr(value, knob.default);
    const unitSuffix = knob.unit ? ' <span class="knob-unit">' + esc(knob.unit) + '</span>' : '';
    return (
      '<input type="range" class="knob-input-range" ' +
        'data-knob-role="range" ' +
        'min="' + esc(String(min)) + '" max="' + esc(String(max)) + '" step="' + esc(String(step)) + '" ' +
        'value="' + esc(String(v)) + '" />' +
      '<input type="number" class="knob-input-number" ' +
        'data-knob-role="number" ' +
        'min="' + esc(String(min)) + '" max="' + esc(String(max)) + '" step="' + esc(String(step)) + '" ' +
        'value="' + esc(String(v)) + '" />' +
      unitSuffix
    );
  }

  function renderEnum(knob, value) {
    const v = typeof value === 'string' ? value : String(value || '');
    const opts = (knob.enum_values || []).map(ev =>
      '<option value="' + esc(ev) + '"' + (ev === v ? ' selected' : '') + '>' + esc(ev) + '</option>'
    ).join('');
    return '<select class="knob-input-select" data-knob-role="enum">' + opts + '</select>';
  }

  function renderString(knob, value) {
    const v = typeof value === 'string' ? value : '';
    const ph = knob.default ? 'default: ' + String(knob.default) : '';
    return '<input type="text" class="knob-input-text" data-knob-role="string" ' +
      'value="' + esc(v) + '" placeholder="' + esc(ph) + '" />';
  }

  // Multiselect: v1 is a comma-separated text input. The catalog doesn't
  // carry a universe of options for allowed_tools (only the default list is
  // in KnobDef.default), so a free-form tag input is the honest shape until
  // M4 ties it to the agent tool registry. Spec explicitly authorizes this
  // trade-off.
  function renderMultiselect(knob, value) {
    const arr = Array.isArray(value) ? value : (Array.isArray(knob.default) ? knob.default : []);
    const csv = arr.map(x => String(x)).join(', ');
    return '<input type="text" class="knob-input-text knob-input-multiselect" ' +
      'data-knob-role="multiselect" ' +
      'value="' + esc(csv) + '" placeholder="comma-separated values" />';
  }

  function renderProvenance(scope, source, sourceID, isExplicit) {
    // Product/manifest scopes don't have an inheritance story to show —
    // their values are either explicit or the system default. Task scope
    // is the only tier where "from manifest X" vs "from product Y" matters.
    if (scope.type !== 'task') {
      if (isExplicit) return '<span class="knob-source explicit">set at ' + esc(scope.type) + '</span>';
      return '<span class="knob-source">system default</span>';
    }
    if (source === 'task') return '<span class="knob-source explicit">set on this task</span>';
    if (source === 'system') return '<span class="knob-source">system default</span>';
    // manifest/product: show the id as a placeholder; hydrateProvenance
    // swaps in the human name once the /api/<type>s/<id> lookup returns.
    const short = sourceID ? sourceID.substring(0, 8) : '';
    return '<span class="knob-source" data-source-type="' + esc(source) + '" data-source-id="' + esc(sourceID) + '">' +
      'from ' + esc(source) + (short ? ' (' + esc(short) + ')' : '') + '</span>';
  }

  function renderWarnings(knob, value) {
    const list = computeWarnings(knob, value);
    if (list.length === 0) return '';
    return list.map(w => '<span class="knob-warning">' + esc(w) + '</span>').join('');
  }

  // --- client-side soft warnings -------------------------------------------

  function computeWarnings(knob, value) {
    const out = [];
    if (knob.key === 'max_parallel' && typeof value === 'number') {
      const cores = (navigator && navigator.hardwareConcurrency) || 0;
      if (cores > 0 && value > cores) {
        out.push('Exceeds CPU count (' + cores + '); tasks will queue');
      }
    }
    if (knob.key === 'temperature' && typeof value === 'number' && value > 1.5) {
      out.push('High temperature rarely helps for coding');
    }
    if (knob.key === 'daily_budget_usd' && typeof value === 'number' && value > 90) {
      out.push('Within $10 of visceral rule cap ($100)');
    }
    return out;
  }

  // --- event wiring --------------------------------------------------------

  function bindHandlers(containerEl, scope, catalog) {
    const catalogByKey = new Map();
    catalog.forEach(k => catalogByKey.set(k.key, k));

    containerEl.querySelectorAll('.knob-row').forEach(row => {
      const key = row.dataset.knobKey;
      const knob = catalogByKey.get(key);
      if (!knob) return;

      const range = row.querySelector('[data-knob-role="range"]');
      const number = row.querySelector('[data-knob-role="number"]');
      const sel = row.querySelector('[data-knob-role="enum"]');
      const txt = row.querySelector('[data-knob-role="string"]');
      const multi = row.querySelector('[data-knob-role="multiselect"]');

      // Range ↔ number mirror the same value live; only the final change
      // flushes to the server (via debouncedSave).
      if (range && number) {
        range.addEventListener('input', () => {
          number.value = range.value;
          refreshWarnings(row, knob, coerceNumber(range.value, knob));
        });
        range.addEventListener('change', () => {
          scheduleSave(scope, knob, coerceNumber(range.value, knob), row);
        });
        number.addEventListener('input', () => {
          range.value = number.value;
          refreshWarnings(row, knob, coerceNumber(number.value, knob));
        });
        number.addEventListener('change', () => {
          scheduleSave(scope, knob, coerceNumber(number.value, knob), row);
        });
      }

      if (sel) {
        sel.addEventListener('change', () => {
          scheduleSave(scope, knob, sel.value, row);
        });
      }

      if (txt) {
        txt.addEventListener('change', () => {
          scheduleSave(scope, knob, txt.value, row);
        });
      }

      if (multi) {
        multi.addEventListener('change', () => {
          const arr = multi.value.split(',').map(s => s.trim()).filter(Boolean);
          scheduleSave(scope, knob, arr, row);
        });
      }

      const resetBtn = row.querySelector('.knob-reset');
      if (resetBtn) {
        resetBtn.addEventListener('click', () => resetKnob(scope, key, containerEl));
      }
    });
  }

  function refreshWarnings(row, knob, value) {
    const meta = row.querySelector('.knob-row-meta');
    if (!meta) return;
    // Keep the existing source line, replace only the warnings.
    meta.querySelectorAll('.knob-warning').forEach(el => el.remove());
    const warnings = computeWarnings(knob, value);
    warnings.forEach(w => {
      const span = document.createElement('span');
      span.className = 'knob-warning';
      span.textContent = w;
      meta.appendChild(span);
    });
  }

  // --- debounced save ------------------------------------------------------

  const _saveTimers = new Map();
  function scheduleSave(scope, knob, value, row) {
    const key = knob.key;
    const existing = _saveTimers.get(key);
    if (existing) clearTimeout(existing);
    _saveTimers.set(key, setTimeout(() => {
      _saveTimers.delete(key);
      doSave(scope, knob, value, row);
    }, 400));
  }

  async function doSave(scope, knob, value, row) {
    const status = row.querySelector('.knob-save-status');
    if (status) { status.textContent = '…'; status.className = 'knob-save-status pending'; }
    try {
      const resp = await fetch('/api/' + scope.type + 's/' + scope.id + '/settings', {
        method: 'PUT',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ [knob.key]: value }),
      });
      const data = await resp.json();
      const result = (data && data.results && data.results[0]) || {ok: false, error: 'unexpected response'};
      if (result.ok) {
        if (status) { status.textContent = '\u2713'; status.className = 'knob-save-status ok'; }
        // Server warnings (e.g. slider range) are additive to client-side ones.
        applyServerWarnings(row, result.warnings || []);
        // Saved values now exist explicitly at this scope — show Reset.
        ensureResetButton(row, scope, knob.key);
        // Provenance line must update to reflect the new explicit status.
        updateProvenanceAfterSave(row, scope);
        setTimeout(() => { if (status) status.textContent = ''; }, 1800);
      } else {
        if (status) { status.textContent = '\u2717'; status.className = 'knob-save-status err'; status.title = result.error || 'save failed'; }
      }
    } catch(e) {
      if (status) { status.textContent = '\u2717'; status.className = 'knob-save-status err'; status.title = e.message || 'network error'; }
    }
  }

  function applyServerWarnings(row, serverWarnings) {
    const meta = row.querySelector('.knob-row-meta');
    if (!meta) return;
    serverWarnings.forEach(w => {
      const span = document.createElement('span');
      span.className = 'knob-warning';
      span.textContent = w;
      meta.appendChild(span);
    });
  }

  function ensureResetButton(row, scope, key) {
    if (row.querySelector('.knob-reset')) return;
    const main = row.querySelector('.knob-row-main');
    if (!main) return;
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'knob-reset';
    btn.dataset.knobKey = key;
    btn.title = 'Reset to inherited value';
    btn.textContent = 'Reset';
    btn.addEventListener('click', () => {
      const container = row.closest('.knob-section') ? row.closest('.knob-section').parentElement : row.parentElement;
      resetKnob(scope, key, container);
    });
    main.appendChild(btn);
  }

  function updateProvenanceAfterSave(row, scope) {
    const meta = row.querySelector('.knob-row-meta');
    if (!meta) return;
    const src = meta.querySelector('.knob-source');
    if (!src) return;
    src.className = 'knob-source explicit';
    src.textContent = scope.type === 'task' ? 'set on this task' : 'set at ' + scope.type;
    src.removeAttribute('data-source-type');
    src.removeAttribute('data-source-id');
  }

  // --- reset (DELETE) ------------------------------------------------------

  async function resetKnob(scope, key, containerEl) {
    try {
      const resp = await fetch('/api/' + scope.type + 's/' + scope.id + '/settings/' + encodeURIComponent(key), {
        method: 'DELETE',
      });
      if (!resp.ok) {
        const txt = await resp.text();
        throw new Error(txt || ('HTTP ' + resp.status));
      }
      // Re-render the whole section so inheritance picks up the new source.
      if (containerEl) {
        await OL.renderKnobSection(containerEl, scope);
      }
    } catch(e) {
      console.error('Reset knob failed:', e);
      alert('Failed to reset ' + key + ': ' + (e.message || e));
    }
  }

  // --- provenance hydration (task scope only) ------------------------------

  async function hydrateProvenance(containerEl, resolvedMap) {
    const sources = containerEl.querySelectorAll('.knob-source[data-source-type]');
    sources.forEach(async el => {
      const type = el.dataset.sourceType;
      const id = el.dataset.sourceId;
      if (!type || !id) return;
      if (type !== 'manifest' && type !== 'product') return;
      const name = await lookupScopeName(type, id);
      if (name && name !== id.substring(0, 8)) {
        el.textContent = 'from ' + type + ' (' + name + ')';
      }
    });
  }

  // --- helpers -------------------------------------------------------------

  function safeParse(jsonStr) {
    try { return JSON.parse(jsonStr); }
    catch(e) { return jsonStr; }
  }

  function numericOr(v, fallback) {
    if (typeof v === 'number' && isFinite(v)) return v;
    if (typeof fallback === 'number' && isFinite(fallback)) return fallback;
    return 0;
  }

  function coerceNumber(str, knob) {
    const n = Number(str);
    if (!isFinite(n)) return 0;
    return knob.type === 'int' ? Math.round(n) : n;
  }

})(window.OL);
