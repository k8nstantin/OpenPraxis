// Shared per-tab search input — manifest 019daafb-b5e M3+M5.
// Renders a debounced <input> + clear-X + results-count badge into `container`.
// Each tab owns its own onSearch / onClear callbacks; this component only
// handles UI + debounce + cancel-on-new-input. 250 ms is locked by M5.
(function(OL) {
  'use strict';

  OL.mountSearchInput = function(container, opts) {
    opts = opts || {};
    var placeholder = opts.placeholder || 'Search...';
    var debounceMs  = opts.debounceMs != null ? opts.debounceMs : 250;
    var ariaLabel   = opts.ariaLabel || placeholder;

    container.innerHTML =
      '<div class="ol-search" style="display:flex;align-items:center;gap:8px;margin-bottom:12px">' +
        '<div style="position:relative;flex:1">' +
          '<input type="text" class="ol-search-input conv-search" ' +
            'placeholder="' + OL.esc(placeholder) + '" ' +
            'aria-label="' + OL.esc(ariaLabel) + '" ' +
            'style="width:100%;padding-right:28px" />' +
          '<button type="button" class="ol-search-clear" aria-label="Clear search" ' +
            'style="display:none;position:absolute;right:6px;top:50%;transform:translateY(-50%);' +
            'background:none;border:none;color:var(--text-muted);font-size:18px;cursor:pointer;' +
            'padding:0 4px;line-height:1">&times;</button>' +
        '</div>' +
        '<span class="ol-search-count" ' +
          'style="display:none;font-size:11px;color:var(--text-muted);' +
          'font-family:var(--font-mono);white-space:nowrap"></span>' +
      '</div>';

    var input   = container.querySelector('.ol-search-input');
    var clearEl = container.querySelector('.ol-search-clear');
    var countEl = container.querySelector('.ol-search-count');
    var timer   = null;
    var lastQ   = '';

    function setCount(n) {
      if (n == null || n < 0) {
        countEl.style.display = 'none';
        countEl.textContent = '';
        return;
      }
      countEl.style.display = '';
      countEl.textContent = n + (n === 1 ? ' result' : ' results');
    }

    function restore() {
      lastQ = '';
      setCount(null);
      if (typeof opts.onClear === 'function') opts.onClear();
    }

    function fire(q) {
      if (!q) { restore(); return; }
      if (typeof opts.onSearch !== 'function') return;
      lastQ = q;
      Promise.resolve(opts.onSearch(q)).then(function(n) {
        if (q !== lastQ) return; // stale — a newer query has taken over
        if (typeof n === 'number') setCount(n);
      }).catch(function(err) {
        console.error('Search failed:', err);
      });
    }

    function doClear() {
      input.value = '';
      if (timer) { clearTimeout(timer); timer = null; }
      clearEl.style.display = 'none';
      restore();
      input.focus();
    }

    OL.onView(input, 'input', function() {
      var q = input.value.trim();
      clearEl.style.display = q ? '' : 'none';
      if (timer) { clearTimeout(timer); timer = null; }
      if (!q) { restore(); return; }
      timer = setTimeout(function() { timer = null; fire(q); }, debounceMs);
    });

    OL.onView(input, 'keydown', function(e) {
      if (e.key === 'Enter') {
        e.preventDefault();
        if (timer) { clearTimeout(timer); timer = null; }
        fire(input.value.trim());
      } else if (e.key === 'Escape') {
        doClear();
      }
    });

    OL.onView(clearEl, 'click', doClear);

    return {
      setCount: setCount,
      clear: doClear,
      focus: function() { input.focus(); },
      value: function() { return input.value.trim(); }
    };
  };

})(window.OL);
