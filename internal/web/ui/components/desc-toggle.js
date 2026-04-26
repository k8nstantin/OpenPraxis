// Description / content view toggle — Markup vs Rendered.
//
// DV/M5 + #238 made agent-authored XML markup render as styled blocks
// in the dashboard. But operators iterating on prompts want to see the
// raw text the agent will receive, NOT the prettified version. This
// component renders both views into the same surface and lets the
// operator flip between them with two buttons; the choice persists
// in localStorage so future loads default to whatever the operator
// last picked.
//
// Default is "markup" (raw). The agent-readable surface is the source
// of truth; the rendered view is a non-techie convenience.
//
// Usage:
//   container.innerHTML = OL.descToggle(rawText, renderedHTML, opts)
// where opts = { id?, className?, style? }.

(function () {
  'use strict';
  var STORAGE_KEY = 'descMode';

  function getMode() {
    try {
      return window.localStorage.getItem(STORAGE_KEY) || 'markup';
    } catch (_) {
      return 'markup';
    }
  }

  function setMode(mode) {
    try {
      window.localStorage.setItem(STORAGE_KEY, mode);
    } catch (_) { /* localStorage disabled — non-fatal */ }
  }

  function descToggle(rawText, renderedHTML, opts) {
    opts = opts || {};
    var id = opts.id || ('desc-' + Math.random().toString(36).slice(2, 9));
    var className = opts.className || 'md-body';
    var style = opts.style || '';
    var mode = getMode();

    // Defensive: when raw is missing, render empty placeholder.
    var raw = rawText == null ? '' : String(rawText);
    var rendered = renderedHTML == null ? '' : String(renderedHTML);

    return ''
      + '<div class="desc-toggle-wrap" data-desc-id="' + id + '">'
      +   '<div class="desc-toggle-controls">'
      +     '<button class="desc-toggle-btn ' + (mode === 'markup' ? 'active' : '') + '" '
      +            'data-mode="markup" data-target="' + id + '" type="button">Markup</button>'
      +     '<button class="desc-toggle-btn ' + (mode === 'rendered' ? 'active' : '') + '" '
      +            'data-mode="rendered" data-target="' + id + '" type="button">Rendered</button>'
      +   '</div>'
      +   '<pre id="' + id + '-markup" class="desc-markup" '
      +        'style="display:' + (mode === 'markup' ? 'block' : 'none') + '">'
      +     OL.esc(raw)
      +   '</pre>'
      +   '<div id="' + id + '-rendered" class="' + className + '" '
      +        'style="display:' + (mode === 'rendered' ? 'block' : 'none') + ';' + style + '">'
      +     rendered
      +   '</div>'
      + '</div>';
  }

  // Single delegated click handler. Survives view re-renders because
  // it lives on document; new toggle wraps inserted later inherit it.
  document.addEventListener('click', function (e) {
    var btn = e.target.closest && e.target.closest('.desc-toggle-btn');
    if (!btn) return;
    var mode = btn.dataset.mode;
    var targetId = btn.dataset.target;
    var wrap = btn.closest('.desc-toggle-wrap');
    if (!wrap || !mode || !targetId) return;

    // Update active state on both buttons in this wrap.
    wrap.querySelectorAll('.desc-toggle-btn').forEach(function (b) {
      b.classList.toggle('active', b.dataset.mode === mode);
    });
    // Show/hide bodies.
    var markupEl = document.getElementById(targetId + '-markup');
    var renderedEl = document.getElementById(targetId + '-rendered');
    if (markupEl) markupEl.style.display = mode === 'markup' ? 'block' : 'none';
    if (renderedEl) renderedEl.style.display = mode === 'rendered' ? 'block' : 'none';

    setMode(mode);

    // If other toggles exist on the page, sync them so the whole
    // dashboard reflects the operator's preferred view.
    document.querySelectorAll('.desc-toggle-wrap').forEach(function (w) {
      if (w === wrap) return;
      var otherId = w.dataset.descId;
      if (!otherId) return;
      w.querySelectorAll('.desc-toggle-btn').forEach(function (b) {
        b.classList.toggle('active', b.dataset.mode === mode);
      });
      var om = document.getElementById(otherId + '-markup');
      var or_ = document.getElementById(otherId + '-rendered');
      if (om) om.style.display = mode === 'markup' ? 'block' : 'none';
      if (or_) or_.style.display = mode === 'rendered' ? 'block' : 'none';
    });
  });

  window.OL = window.OL || {};
  window.OL.descToggle = descToggle;
})();
