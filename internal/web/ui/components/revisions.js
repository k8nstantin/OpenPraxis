// OpenPraxis — Description Revisions component (DV/M5)
//
// Fetches /api/{type}s/:id/description/history and renders a collapsible
// History section with per-row "View diff" and "Restore" buttons. Shared
// across product / manifest / task detail views.
//
// Integration:
//   OL.renderRevisionsSection(containerEl, { type: 'product'|'manifest'|'task', id: '<id>' })

(function (OL) {
  'use strict';

  var fetchJSON = OL.fetchJSON, esc = OL.esc, timeAgo = OL.timeAgo;

  // ---- LCS line diff --------------------------------------------------------
  // Tiny line-level diff, ~O(n·m). Adequate for description bodies (tens to
  // hundreds of lines); not for large files. Returns an array of
  // { tag: 'eq'|'add'|'del', line: '...' } rows matching unified-diff order.
  function diffLines(oldText, newText) {
    var a = (oldText || '').split('\n');
    var b = (newText || '').split('\n');
    var n = a.length, m = b.length;
    var dp = new Array(n + 1);
    for (var i = 0; i <= n; i++) {
      dp[i] = new Int32Array(m + 1);
    }
    for (i = n - 1; i >= 0; i--) {
      for (var j = m - 1; j >= 0; j--) {
        dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
      }
    }
    var out = [];
    i = 0; j = 0;
    while (i < n && j < m) {
      if (a[i] === b[j]) {
        out.push({ tag: 'eq', line: a[i] });
        i++; j++;
      } else if (dp[i + 1][j] >= dp[i][j + 1]) {
        out.push({ tag: 'del', line: a[i] });
        i++;
      } else {
        out.push({ tag: 'add', line: b[j] });
        j++;
      }
    }
    while (i < n) { out.push({ tag: 'del', line: a[i++] }); }
    while (j < m) { out.push({ tag: 'add', line: b[j++] }); }
    return out;
  }

  function renderDiff(oldBody, newBody) {
    var rows = diffLines(oldBody, newBody);
    if (!rows.length) return '<div class="diff-empty">(no changes)</div>';
    var html = rows.map(function (r) {
      var cls = r.tag === 'add' ? 'diff-add' : r.tag === 'del' ? 'diff-del' : 'diff-eq';
      var prefix = r.tag === 'add' ? '+ ' : r.tag === 'del' ? '- ' : '  ';
      return '<div class="diff-line ' + cls + '">' + esc(prefix + r.line) + '</div>';
    }).join('');
    return '<pre class="diff-pre">' + html + '</pre>';
  }

  // ---- modal ---------------------------------------------------------------

  function openModal(title, bodyHtml) {
    var existing = document.getElementById('ol-revision-modal');
    if (existing) existing.remove();
    var modal = document.createElement('div');
    modal.id = 'ol-revision-modal';
    modal.className = 'revision-modal-backdrop';
    modal.innerHTML =
      '<div class="revision-modal">' +
        '<div class="revision-modal-header">' +
          '<span class="revision-modal-title">' + esc(title) + '</span>' +
          '<button type="button" class="revision-modal-close" aria-label="Close">&times;</button>' +
        '</div>' +
        '<div class="revision-modal-body">' + bodyHtml + '</div>' +
      '</div>';
    document.body.appendChild(modal);
    var close = function () { modal.remove(); document.removeEventListener('keydown', onKey); };
    var onKey = function (e) { if (e.key === 'Escape') close(); };
    modal.querySelector('.revision-modal-close').onclick = close;
    modal.addEventListener('click', function (e) { if (e.target === modal) close(); });
    document.addEventListener('keydown', onKey);
  }

  // ---- entry point ---------------------------------------------------------

  OL.renderRevisionsSection = function (containerEl, scope) {
    if (!containerEl || !scope || !scope.type || !scope.id) return;

    var base = '/api/' + scope.type + 's/' + encodeURIComponent(scope.id) + '/description';
    containerEl.innerHTML = '<div class="revisions-loading">Loading history…</div>';

    function load() {
      return fetchJSON(base + '/history?limit=100').then(function (resp) {
        return (resp && resp.items) || [];
      });
    }

    function paint(items) {
      if (!items.length) {
        containerEl.innerHTML =
          '<div class="revisions-section">' +
            '<div class="revisions-header">History</div>' +
            '<div class="revisions-empty">No revisions yet. Edits to this entity\'s description will show up here.</div>' +
          '</div>';
        return;
      }
      var rows = items.map(function (r, idx) {
        var isNewest = idx === 0;
        var preview = (r.body || '').split('\n')[0] || '(empty)';
        if (preview.length > 140) preview = preview.slice(0, 140) + '…';
        var currentBadge = isNewest ? '<span class="revision-current-badge">current</span>' : '';
        var actions =
          '<button type="button" class="revision-btn revision-diff-btn" data-idx="' + idx + '">View diff</button>' +
          (isNewest ? '' : '<button type="button" class="revision-btn revision-restore-btn" data-idx="' + idx + '">Restore</button>');
        return (
          '<div class="revision-row" data-idx="' + idx + '">' +
            '<div class="revision-row-meta">' +
              '<span class="revision-version">v' + r.version + '</span>' +
              '<span class="revision-author">' + esc(r.author) + '</span>' +
              '<span class="revision-time">' + esc(timeAgo(new Date(r.created_at * 1000).toISOString())) + '</span>' +
              currentBadge +
            '</div>' +
            '<div class="revision-preview">' + esc(preview) + '</div>' +
            '<div class="revision-actions">' + actions + '</div>' +
          '</div>'
        );
      }).join('');
      containerEl.innerHTML =
        '<div class="revisions-section">' +
          '<div class="revisions-header">History <span class="revisions-count">(' + items.length + ')</span></div>' +
          '<div class="revisions-list">' + rows + '</div>' +
        '</div>';
      bind(items);
    }

    function bind(items) {
      containerEl.querySelectorAll('.revision-diff-btn').forEach(function (btn) {
        btn.onclick = function () {
          var i = Number(btn.getAttribute('data-idx'));
          var cur = items[i];
          // Diff vs the next-older revision. For the oldest row, diff
          // against empty — shows every line as an addition.
          var prev = items[i + 1];
          var oldBody = prev ? prev.body : '';
          var newBody = cur ? cur.body : '';
          var title = 'v' + cur.version + (prev ? ' vs v' + prev.version : ' (initial)');
          openModal(title, renderDiff(oldBody, newBody));
        };
      });
      containerEl.querySelectorAll('.revision-restore-btn').forEach(function (btn) {
        btn.onclick = function () {
          var i = Number(btn.getAttribute('data-idx'));
          var r = items[i];
          if (!confirm('Restore v' + r.version + '?\n\nThis creates a new revision matching the historical body. The current body will be replaced.')) return;
          var author = (OL.currentUser && OL.currentUser()) || 'operator';
          fetch(base + '/restore/' + encodeURIComponent(r.id), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ author: author }),
          }).then(function (resp) {
            return resp.json().then(function (data) { return { ok: resp.ok, data: data }; });
          }).then(function (res) {
            if (!res.ok) throw new Error((res.data && res.data.error) || 'restore failed');
            // Reload history inline. Parent view reloads on its own cadence
            // — the denormalised description column is what callers see
            // and the next view switch will pull the new body.
            return load().then(paint);
          }).catch(function (e) { alert(e.message || e); });
        };
      });
    }

    load().then(paint).catch(function (e) {
      containerEl.innerHTML = '<div class="revisions-error">Failed to load history: ' + esc(e.message || String(e)) + '</div>';
    });
  };
})(window.OL);
