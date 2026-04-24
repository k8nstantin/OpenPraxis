(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, timeAgo = OL.timeAgo;

  // Persists selected template_uid across view-switch round-trips.
  var _selectedUID = null;

  // --- Minimal markdown → HTML -------------------------------------------
  function escHtml(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function inlineFmt(text) {
    var t = escHtml(text);
    // inline code first (protect from bold/italic pass)
    t = t.replace(/`([^`\n]+)`/g, '<code>$1</code>');
    t = t.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>');
    t = t.replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
    return t;
  }

  function mdToHtml(text) {
    if (!text) return '';
    var lines = text.split('\n');
    var out = [];
    var inCode = false;
    var codeLines = [];
    var inList = false;

    function closeList() {
      if (inList) { out.push('</ul>'); inList = false; }
    }

    for (var i = 0; i < lines.length; i++) {
      var line = lines[i];
      var lTrim = line.replace(/^\s+/, '');

      // Fenced code blocks
      if (lTrim.startsWith('```')) {
        if (inCode) {
          out.push('<pre><code>' + escHtml(codeLines.join('\n')) + '</code></pre>');
          codeLines = [];
          inCode = false;
        } else {
          closeList();
          inCode = true;
        }
        continue;
      }
      if (inCode) { codeLines.push(line); continue; }

      // List items (-, *, or 1.)
      if (/^[-*] /.test(line) || /^\d+\. /.test(line)) {
        if (!inList) { out.push('<ul>'); inList = true; }
        var content = line.replace(/^[-*] /, '').replace(/^\d+\. /, '');
        out.push('<li>' + inlineFmt(content) + '</li>');
        continue;
      }
      closeList();

      // Headings
      var hm = line.match(/^(#{1,4}) (.*)/);
      if (hm) {
        var hl = hm[1].length;
        out.push('<h' + hl + '>' + inlineFmt(hm[2]) + '</h' + hl + '>');
        continue;
      }

      // Blank line — skip (CSS handles spacing)
      if (!line.trim()) continue;

      out.push('<p>' + inlineFmt(line) + '</p>');
    }

    closeList();
    if (inCode && codeLines.length) {
      out.push('<pre><code>' + escHtml(codeLines.join('\n')) + '</code></pre>');
    }
    return out.join('\n');
  }

  // --- Left pane: scope tree --------------------------------------------
  OL.loadRunner = async function() {
    var listEl = document.getElementById('runner-list');
    if (!listEl) return;

    listEl.innerHTML = '<div class="empty-state" style="padding:16px">Loading...</div>';

    try {
      var results = await Promise.all([
        fetchJSON('/api/templates?scope=system'),
        fetchJSON('/api/templates?scope=product'),
        fetchJSON('/api/templates?scope=manifest'),
        fetchJSON('/api/templates?scope=task'),
        fetchJSON('/api/templates?scope=agent'),
      ]);

      var scopes = [
        { label: 'system',            items: results[0] || [] },
        { label: 'product overrides',  items: results[1] || [] },
        { label: 'manifest overrides', items: results[2] || [] },
        { label: 'task overrides',     items: results[3] || [] },
        { label: 'agent overrides',    items: results[4] || [] },
      ];

      OL.renderTree(listEl, scopes, {
        prefix: 'runner',
        emptyMessage: 'No templates found',
        levels: [
          {
            label: function(g) { return esc(g.label); },
            count: function(g) { return g.items.length || ''; },
            children: function(g) { return g.items; },
            expanded: false,
            dotColor: '',
          }
        ],
        renderLeaf: function(t) {
          var active = t.template_uid === _selectedUID ? ' active' : '';
          var badge = t.scope !== 'system' && t.scope_id
            ? '<span class="runner-scope-badge">' + esc(t.scope_id.slice(0, 8)) + '</span>'
            : '';
          return '<div class="runner-template-row tree-leaf' + active + '"' +
            ' data-uid="' + esc(t.template_uid) + '"' +
            ' role="button" tabindex="0">' +
            '<span class="runner-section-name">' + esc(t.section) + '</span>' +
            badge +
            '</div>';
        },
        leafSelector: '.runner-template-row',
        onLeafClick: function(el) {
          listEl.querySelectorAll('.runner-template-row').forEach(function(r) {
            r.classList.remove('active');
          });
          el.classList.add('active');
          OL.loadTemplate(el.dataset.uid);
        },
        afterRender: function(container) {
          // Expand system group (first group, index 0) by default
          var fc = container.querySelector('[data-runner-l0-children="0"]');
          var fh = container.querySelector('[data-runner-l0="0"]');
          if (fc) fc.style.display = '';
          if (fh) {
            var arrow = fh.querySelector('.tree-arrow');
            if (arrow) arrow.innerHTML = '▼';
            fh.setAttribute('aria-expanded', 'true');
          }
          // Restore selection highlight after re-render
          if (_selectedUID) {
            var sel = container.querySelector('[data-uid="' + CSS.escape(_selectedUID) + '"]');
            if (sel) sel.classList.add('active');
          }
        }
      });
    } catch (e) {
      listEl.innerHTML = '<div class="empty-state" style="padding:16px">Failed to load templates</div>';
      console.error('loadRunner:', e);
    }
  };

  // --- Right pane: template detail + history ----------------------------
  OL.loadTemplate = async function(uid) {
    _selectedUID = uid;
    var detailEl = document.getElementById('runner-detail');
    var titleEl  = document.getElementById('runner-detail-title');
    if (!detailEl) return;

    detailEl.innerHTML = '<div class="empty-state">Loading...</div>';

    try {
      var results = await Promise.all([
        fetchJSON('/api/templates/' + encodeURIComponent(uid)),
        fetchJSON('/api/templates/' + encodeURIComponent(uid) + '/history'),
      ]);
      var tpl     = results[0];
      var history = results[1] || [];

      if (titleEl) titleEl.textContent = tpl.title || tpl.section;

      var lastEdit = history.length > 0 ? history[0] : tpl;

      // Metadata bar
      var metaHtml =
        '<div class="runner-meta-bar">' +
          '<span class="runner-title">' + esc(tpl.title || tpl.section) + '</span>' +
          '<span class="runner-status-badge runner-status-' + esc(tpl.status) + '">' + esc(tpl.status) + '</span>' +
          '<span class="runner-meta-item">' +
            '<span class="runner-meta-label">Last edit</span>' +
            '<span>' + esc(lastEdit.changed_by || '—') + '</span>' +
            '<span style="color:var(--text-muted)">&middot;</span>' +
            '<span>' + (lastEdit.valid_from ? timeAgo(lastEdit.valid_from) : '—') + '</span>' +
            (lastEdit.reason
              ? '<span class="runner-reason-inline">' + esc(lastEdit.reason) + '</span>'
              : '') +
          '</span>' +
        '</div>';

      // Current body (rendered markdown)
      var bodyHtml =
        '<div class="runner-section-header">Current body</div>' +
        '<div class="md-body runner-body-block">' + mdToHtml(tpl.body) + '</div>';

      // History list newest-first
      var total = history.length;
      var histHtml = '<div class="runner-section-header">History</div>';
      if (!total) {
        histHtml += '<div class="empty-state" style="margin:8px 0">No history</div>';
      } else {
        histHtml += history.map(function(row, idx) {
          var ver = total - idx;
          return '<div class="runner-template-row runner-history-row">' +
            '<span class="runner-ver">v' + ver + '</span>' +
            '<span class="runner-author">' + esc(row.changed_by || '—') + '</span>' +
            '<span class="runner-time">' + (row.valid_from ? timeAgo(row.valid_from) : '—') + '</span>' +
            (row.reason
              ? '<span class="runner-reason">' + esc(row.reason) + '</span>'
              : '') +
            '</div>';
        }).join('');
      }

      detailEl.innerHTML = metaHtml + bodyHtml + histHtml;
    } catch (e) {
      detailEl.innerHTML = '<div class="empty-state">Failed to load template</div>';
      console.error('loadTemplate:', e);
    }
  };

})(window.OL);
