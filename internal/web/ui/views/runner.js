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

  // --- Editor state (one editor in flight at a time) --------------------
  var _editorMDE = null;
  var _editorDraft = null;
  var _editorTpl = null;

  function closeEditor() {
    if (_editorMDE) { try { _editorMDE.detach(); } catch (e) {} _editorMDE = null; }
    _editorDraft = null;
    _editorTpl = null;
  }

  // --- Restore confirm modal --------------------------------------------
  function showRestoreConfirm(ver, ts, onConfirm) {
    var existing = document.getElementById('runner-restore-modal');
    if (existing) existing.remove();
    var modal = document.createElement('div');
    modal.id = 'runner-restore-modal';
    modal.className = 'runner-restore-confirm';
    modal.innerHTML =
      '<div class="runner-restore-confirm-box">' +
        '<div class="runner-restore-confirm-title">Restore revision</div>' +
        '<div class="runner-restore-confirm-body">This creates a new revision matching v' +
          esc(String(ver)) + ' from ' + esc(ts) + '. Proceed?</div>' +
        '<div class="runner-restore-confirm-actions">' +
          '<button type="button" class="btn-secondary" data-action="cancel">Cancel</button>' +
          '<button type="button" class="btn-primary" data-action="confirm">Confirm</button>' +
        '</div>' +
      '</div>';
    document.body.appendChild(modal);
    modal.querySelector('[data-action="cancel"]').addEventListener('click', function() {
      modal.remove();
    });
    modal.querySelector('[data-action="confirm"]').addEventListener('click', function() {
      modal.remove();
      onConfirm();
    });
  }

  // --- Right pane: template detail + history ----------------------------
  OL.loadTemplate = async function(uid) {
    _selectedUID = uid;
    closeEditor();
    var detailEl = document.getElementById('runner-detail');
    var titleEl  = document.getElementById('runner-detail-title');
    if (!detailEl) return;

    detailEl.innerHTML = '<div class="empty-state">Loading...</div>';

    try {
      var results = await Promise.all([
        fetchJSON('/api/templates/' + encodeURIComponent(uid)),
        fetchJSON('/api/templates/' + encodeURIComponent(uid) + '/history'),
        fetchJSON('/api/tasks?limit=200').catch(function() { return []; }),
      ]);
      var tpl     = results[0];
      var history = results[1] || [];
      var tasks   = results[2] || [];

      if (titleEl) titleEl.textContent = tpl.title || tpl.section;
      _editorTpl = tpl;

      var lastEdit = history.length > 0 ? history[0] : tpl;

      // Metadata bar with Edit button
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
          '<button type="button" class="btn-secondary runner-edit-btn" data-action="edit">Edit</button>' +
        '</div>';

      // Current body (rendered markdown) — read-only by default
      var bodyHtml =
        '<div class="runner-section-header">Current body</div>' +
        '<div class="md-body runner-body-block" id="runner-body-block">' + mdToHtml(tpl.body) + '</div>' +
        '<div id="runner-editor-host"></div>';

      // Preview pane
      var taskOpts = '<option value="">— select a task —</option>' +
        (tasks || []).map(function(t) {
          var label = (t.title || t.id || '').slice(0, 80);
          return '<option value="' + esc(t.id) + '">' + esc(label) + '</option>';
        }).join('');
      var previewHtml =
        '<div class="runner-section-header">Preview against task</div>' +
        '<div class="runner-preview-row">' +
          '<select id="runner-preview-task" class="runner-preview-select">' + taskOpts + '</select>' +
        '</div>' +
        '<div id="runner-preview-pane" class="runner-diff-pane runner-preview-pane"></div>';

      // History list newest-first, each row with Restore button
      var total = history.length;
      var histHtml = '<div class="runner-section-header">History</div>';
      if (!total) {
        histHtml += '<div class="empty-state" style="margin:8px 0">No history</div>';
      } else {
        histHtml += history.map(function(row, idx) {
          var ver = total - idx;
          var ts  = row.valid_from || '';
          return '<div class="runner-template-row runner-history-row">' +
            '<span class="runner-ver">v' + ver + '</span>' +
            '<span class="runner-author">' + esc(row.changed_by || '—') + '</span>' +
            '<span class="runner-time" title="' + esc(ts) + '">' +
              (ts ? timeAgo(ts) : '—') + '</span>' +
            (row.reason
              ? '<span class="runner-reason">' + esc(row.reason) + '</span>'
              : '') +
            (idx === 0
              ? ''
              : '<button type="button" class="btn-link runner-restore-btn"' +
                  ' data-ts="' + esc(ts) + '" data-ver="' + esc(String(ver)) + '">Restore</button>') +
            '</div>';
        }).join('');
      }

      detailEl.innerHTML = metaHtml + bodyHtml + previewHtml + histHtml;

      // Bind Edit button
      var editBtn = detailEl.querySelector('[data-action="edit"]');
      if (editBtn) editBtn.addEventListener('click', function() { mountTemplateEditor(uid); });

      // Bind preview select
      var previewSel = detailEl.querySelector('#runner-preview-task');
      if (previewSel) previewSel.addEventListener('change', function() {
        runPreview(uid, previewSel.value);
      });

      // Bind restore buttons
      detailEl.querySelectorAll('.runner-restore-btn').forEach(function(btn) {
        btn.addEventListener('click', function() {
          var ts  = btn.dataset.ts;
          var ver = btn.dataset.ver;
          showRestoreConfirm(ver, ts, function() { doRestore(uid, ts); });
        });
      });
    } catch (e) {
      detailEl.innerHTML = '<div class="empty-state">Failed to load template</div>';
      console.error('loadTemplate:', e);
    }
  };

  // --- Editor mount ------------------------------------------------------
  function mountTemplateEditor(uid) {
    var host = document.getElementById('runner-editor-host');
    var bodyBlock = document.getElementById('runner-body-block');
    if (!host || !_editorTpl) return;
    if (_editorMDE) return; // already editing
    if (bodyBlock) bodyBlock.style.display = 'none';

    host.innerHTML =
      '<div class="runner-editor-row">' +
        '<textarea id="runner-editor-textarea" class="runner-editor-textarea" rows="14">' +
          escHtml(_editorTpl.body || '') +
        '</textarea>' +
        '<div class="runner-reason-row">' +
          '<label class="runner-reason-label" for="runner-editor-reason">Reason</label>' +
          '<input id="runner-editor-reason" class="runner-reason-input" type="text"' +
            ' placeholder="Why are you changing this?" />' +
        '</div>' +
        '<div class="runner-editor-actions">' +
          '<button type="button" class="btn-secondary" data-action="cancel-edit">Cancel</button>' +
          '<button type="button" class="btn-primary" data-action="save-edit" disabled' +
            ' title="Enter a reason to enable Save">Save</button>' +
        '</div>' +
      '</div>';

    var ta     = document.getElementById('runner-editor-textarea');
    var reason = document.getElementById('runner-editor-reason');
    var save   = host.querySelector('[data-action="save-edit"]');
    var cancel = host.querySelector('[data-action="cancel-edit"]');

    _editorMDE = OL.mountEditor(ta, {
      placeholder: 'Template body (markdown + text/template actions)…',
      onSave: function() { if (!save.disabled) save.click(); },
      onCancel: function() { cancel.click(); },
    });
    _editorMDE.focus();

    function refreshSave() {
      var has = reason.value.trim().length > 0;
      save.disabled = !has;
      save.title = has ? '' : 'Enter a reason to enable Save';
    }
    reason.addEventListener('input', refreshSave);
    refreshSave();

    cancel.addEventListener('click', function() {
      closeEditor();
      if (bodyBlock) bodyBlock.style.display = '';
      host.innerHTML = '';
    });

    save.addEventListener('click', async function() {
      var body = _editorMDE ? _editorMDE.value() : ta.value;
      var r    = reason.value.trim();
      if (!r) return;
      save.disabled = true;
      try {
        var resp = await fetch('/api/templates/' + encodeURIComponent(uid), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ body: body, changed_by: 'dashboard', reason: r }),
        });
        if (!resp.ok) {
          var txt = await resp.text();
          alert('Save failed: ' + resp.status + ' ' + txt);
          save.disabled = false;
          return;
        }
        await OL.loadTemplate(uid);
      } catch (e) {
        alert('Save failed: ' + e.message);
        save.disabled = false;
      }
    });
  }

  // --- Preview -----------------------------------------------------------
  async function runPreview(uid, taskID) {
    var pane = document.getElementById('runner-preview-pane');
    if (!pane) return;
    if (!taskID) { pane.innerHTML = ''; return; }
    pane.textContent = 'Rendering…';
    try {
      var resp = await fetch('/api/templates/preview?template_uid=' +
        encodeURIComponent(uid) + '&task_id=' + encodeURIComponent(taskID));
      if (!resp.ok) {
        var txt = await resp.text();
        pane.textContent = '[render error: HTTP ' + resp.status + ' ' + txt + ']';
        return;
      }
      var data = await resp.json();
      var out = (data && data.rendered) || '';
      var pre = document.createElement('pre');
      pre.className = 'runner-preview-output';
      pre.textContent = out;
      pane.innerHTML = '';
      pane.appendChild(pre);
    } catch (e) {
      pane.textContent = '[render error: ' + e.message + ']';
    }
  }

  // --- Restore -----------------------------------------------------------
  async function doRestore(uid, ts) {
    try {
      var resp = await fetch('/api/templates/' + encodeURIComponent(uid) +
        '/restore?from_valid_from=' + encodeURIComponent(ts) +
        '&changed_by=dashboard', { method: 'POST' });
      if (!resp.ok) {
        var txt = await resp.text();
        alert('Restore failed: ' + resp.status + ' ' + txt);
        return;
      }
      await OL.loadTemplate(uid);
    } catch (e) {
      alert('Restore failed: ' + e.message);
    }
  }

})(window.OL);
