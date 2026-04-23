// OpenPraxis — Entity Comments component (M3-T6)
//
// Shared UI for product, manifest, and task detail views. Consumes the
// HTTP surface shipped by M2-T5 + the /api/comments/types endpoint added
// in M3-T6. Markdown is rendered server-side (goldmark); we inject
// response.body_html directly into the DOM.
//
// Integration (M3-T7/T8) calls:
//   OL.renderCommentsSection(containerEl, { type: 'product'|'manifest'|'task', id: '<id>' })

(function(OL) {
  'use strict';

  var fetchJSON = OL.fetchJSON, esc = OL.esc, timeAgo = OL.timeAgo;

  // --- type catalog cache (static per server process) --------------------
  var _typesCache = null;
  function loadTypes() {
    if (_typesCache) return Promise.resolve(_typesCache);
    return fetchJSON('/api/comments/types').then(function(data) {
      _typesCache = Array.isArray(data) ? data : [];
      return _typesCache;
    }).catch(function() {
      // Fallback — keep the dropdown usable even if the endpoint 500s.
      _typesCache = [
        { type: 'user_note', label: 'Note' },
        { type: 'execution_review', label: 'Execution Review' },
        { type: 'watcher_finding', label: 'Watcher Finding' },
        { type: 'agent_note', label: 'Agent Note' },
        { type: 'decision', label: 'Decision' },
        { type: 'link', label: 'Link' },
      ];
      return _typesCache;
    });
  }
  OL.invalidateCommentTypesCache = function() { _typesCache = null; };

  // --- current user (best-effort) ----------------------------------------
  // Used to decide whether to expose edit/delete buttons. Falls back to
  // showing controls for all comments — M3 does not wire up auth UI and
  // the server is the source of truth for authorization.
  function currentAuthor() {
    return (OL.currentUser && OL.currentUser()) || '';
  }

  // --- rendering ---------------------------------------------------------

  function renderAddForm(scope, types) {
    var opts = types.map(function(t) {
      var sel = t.type === 'user_note' ? ' selected' : '';
      return '<option value="' + esc(t.type) + '"' + sel + '>' + esc(t.label) + '</option>';
    }).join('');
    return (
      '<form class="comment-add-form" data-scope-type="' + esc(scope.type) + '" data-scope-id="' + esc(scope.id) + '">' +
        '<textarea class="comment-add-body" placeholder="Add a comment… (Markdown supported)" required></textarea>' +
        '<div class="comment-add-row">' +
          '<select class="comment-add-type">' + opts + '</select>' +
          '<button type="submit" class="comment-add-submit">Post</button>' +
        '</div>' +
        '<div class="comment-add-error" hidden></div>' +
      '</form>'
    );
  }

  function renderFilter(types, currentFilter) {
    var opts = ['<option value="">All</option>'];
    types.forEach(function(t) {
      var sel = t.type === currentFilter ? ' selected' : '';
      opts.push('<option value="' + esc(t.type) + '"' + sel + '>' + esc(t.label) + '</option>');
    });
    return (
      '<div class="comment-filter-row">' +
        '<label class="comment-filter-label">Filter:</label>' +
        '<select class="comment-filter">' + opts.join('') + '</select>' +
      '</div>'
    );
  }

  function renderComment(c) {
    var typeClass = 'comment-type-' + esc(c.type);
    var iso = c.updated_at_iso || c.created_at_iso;
    var relTime = timeAgo(iso) + (c.updated_at_iso ? ' (edited)' : '');
    var me = currentAuthor();
    var canModify = !me || me === c.author;
    var controls = '';
    if (canModify) {
      controls = (
        '<div class="comment-controls">' +
          '<button type="button" class="comment-edit" data-id="' + esc(c.id) + '" title="Edit">✎</button>' +
          '<button type="button" class="comment-delete" data-id="' + esc(c.id) + '" title="Delete">🗑</button>' +
        '</div>'
      );
    }
    // body_html is goldmark-rendered (raw HTML escaped, GFM extensions on);
    // safe to innerHTML-inject.
    return (
      '<div class="comment-card" data-id="' + esc(c.id) + '" data-type="' + esc(c.type) + '">' +
        '<div class="comment-header">' +
          '<span class="comment-author">' + esc(c.author) + '</span>' +
          '<span class="comment-type-badge ' + typeClass + '">' + esc(c.type) + '</span>' +
          '<span class="comment-time" title="' + esc(iso) + '">' + esc(relTime) + '</span>' +
          controls +
        '</div>' +
        '<div class="comment-body">' + (c.body_html || esc(c.body) || '') + '</div>' +
      '</div>'
    );
  }

  function renderStream(comments) {
    if (!comments.length) {
      return '<div class="comment-empty">No comments yet</div>';
    }
    return '<div class="comment-stream">' + comments.map(renderComment).join('') + '</div>';
  }

  // --- entry point -------------------------------------------------------

  OL.renderCommentsSection = function(containerEl, scope) {
    if (!containerEl || !scope || !scope.type || !scope.id) return;
    containerEl.innerHTML = '<div class="comment-section-loading">Loading comments…</div>';

    var state = { scope: scope, types: [], comments: [], filter: '' };

    function url(path) {
      return '/api/' + scope.type + 's/' + encodeURIComponent(scope.id) + '/comments' + (path || '');
    }

    function fetchList() {
      var q = state.filter ? '?limit=50&type=' + encodeURIComponent(state.filter) : '?limit=50';
      return fetchJSON(url(q)).then(function(resp) {
        state.comments = (resp && resp.comments) || [];
      });
    }

    function paint() {
      containerEl.innerHTML = (
        '<div class="comment-section">' +
          '<div class="comment-section-header">Comments</div>' +
          renderAddForm(state.scope, state.types) +
          renderFilter(state.types, state.filter) +
          '<div class="comment-section-stream">' + renderStream(state.comments) + '</div>' +
        '</div>'
      );
      bind();
    }

    function showAddError(msg) {
      var el = containerEl.querySelector('.comment-add-error');
      if (!el) return;
      if (!msg) { el.hidden = true; el.textContent = ''; return; }
      el.hidden = false;
      el.textContent = msg;
    }

    function bind() {
      var form = containerEl.querySelector('.comment-add-form');
      var bodyTa = form && form.querySelector('.comment-add-body');
      var bodyMDE = null;
      // Mount EasyMDE on the composer textarea — compact toolbar so the
      // composer stays small. Detached + remounted on each paint() because
      // the form HTML is rebuilt.
      if (bodyTa && OL.mountEditor) {
        bodyMDE = OL.mountEditor(bodyTa, {
          placeholder: 'Add a comment… (Markdown supported)',
          compact: true,
          autofocus: false,
        });
      }
      if (form) {
        form.onsubmit = function(ev) {
          ev.preventDefault();
          showAddError('');
          var body = bodyMDE ? bodyMDE.value() : bodyTa.value;
          var type = form.querySelector('.comment-add-type').value;
          var author = currentAuthor() || 'user';
          fetch(url(), {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ author: author, type: type, body: body }),
          }).then(function(resp) {
            return resp.json().then(function(data) { return { ok: resp.ok, data: data }; });
          }).then(function(r) {
            if (!r.ok) {
              var code = (r.data && r.data.code) || '';
              var msg = (r.data && r.data.error) || 'Failed to post comment';
              if (code === 'empty_body') msg = 'Comment body is required';
              showAddError(msg);
              return;
            }
            // Prepend on success.
            state.comments.unshift(r.data);
            if (bodyMDE) bodyMDE.setValue('');
            else if (bodyTa) bodyTa.value = '';
            var streamEl = containerEl.querySelector('.comment-section-stream');
            if (streamEl) streamEl.innerHTML = renderStream(state.comments);
            bindStreamHandlers();
          }).catch(function(e) {
            showAddError(String(e && e.message || e));
          });
        };
      }

      var filterEl = containerEl.querySelector('.comment-filter');
      if (filterEl) {
        filterEl.onchange = function() {
          state.filter = filterEl.value;
          fetchList().then(function() {
            var streamEl = containerEl.querySelector('.comment-section-stream');
            if (streamEl) streamEl.innerHTML = renderStream(state.comments);
            bindStreamHandlers();
          });
        };
      }

      bindStreamHandlers();
    }

    function bindStreamHandlers() {
      var stream = containerEl.querySelector('.comment-section-stream');
      if (!stream) return;
      stream.onclick = function(ev) {
        var delBtn = ev.target.closest('.comment-delete');
        var editBtn = ev.target.closest('.comment-edit');
        if (delBtn) {
          var id = delBtn.getAttribute('data-id');
          if (!confirm('Delete this comment?')) return;
          fetch('/api/comments/' + encodeURIComponent(id), { method: 'DELETE' })
            .then(function(resp) {
              if (!resp.ok) throw new Error('delete failed');
              state.comments = state.comments.filter(function(c) { return c.id !== id; });
              stream.innerHTML = renderStream(state.comments);
              bindStreamHandlers();
            }).catch(function(e) { alert(e.message || e); });
          return;
        }
        if (editBtn) {
          var eid = editBtn.getAttribute('data-id');
          var existing = state.comments.find(function(c) { return c.id === eid; });
          if (!existing) return;
          var next = prompt('Edit comment:', existing.body);
          if (next == null || next === existing.body) return;
          fetch('/api/comments/' + encodeURIComponent(eid), {
            method: 'PATCH',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ body: next }),
          }).then(function(resp) {
            return resp.json().then(function(data) { return { ok: resp.ok, data: data }; });
          }).then(function(r) {
            if (!r.ok) throw new Error((r.data && r.data.error) || 'edit failed');
            state.comments = state.comments.map(function(c) { return c.id === eid ? r.data : c; });
            stream.innerHTML = renderStream(state.comments);
            bindStreamHandlers();
          }).catch(function(e) { alert(e.message || e); });
        }
      };
    }

    // Initial load: types + comments in parallel.
    Promise.all([loadTypes(), fetchList()]).then(function(res) {
      state.types = res[0] || [];
      paint();
    }).catch(function(e) {
      containerEl.innerHTML = '<div class="comment-section-error">Failed to load comments: ' + esc(e.message || String(e)) + '</div>';
    });
  };

})(window.OL);
