(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime, formatModel = OL.formatModel;

  // Infinite-scroll state for the conversations keyword search. Reset on
  // every new query via onSearch, appended to on scroll-bottom.
  var convSearch = {
    query: '',
    offset: 0,
    limit: 50,
    total: 0,
    hasMore: false,
    loading: false,
    scrollHandler: null
  };

  function setupConvSearch() {
    var mount = document.getElementById('conv-search-mount');
    if (!mount || !OL.mountSearchInput) return;
    OL.mountSearchInput(mount, {
      placeholder: 'Search conversations by id, marker, or keyword...',
      onSearch: async function(q) {
        convSearch.query = q;
        convSearch.offset = 0;
        convSearch.total = 0;
        convSearch.hasMore = false;
        var env = await fetchConvSearchPage(q, 0, convSearch.limit);
        renderConversationSearchEnvelope(env, /*append=*/ false);
        wireConvSearchScroll();
        return (env.items || []).length + (env.semantic || []).length;
      },
      onClear: function() {
        detachConvSearchScroll();
        OL.loadConversations();
      }
    });
  }

  async function fetchConvSearchPage(q, offset, limit) {
    var url = '/api/conversations/search?q=' + encodeURIComponent(q) +
              '&offset=' + offset + '&limit=' + limit;
    var resp = await fetch(url);
    var env = await resp.json();
    convSearch.total = env.total || 0;
    convSearch.offset = (env.offset || 0) + (env.items || []).length;
    convSearch.hasMore = !!env.has_more;
    return env;
  }

  function wireConvSearchScroll() {
    detachConvSearchScroll();
    var el = document.getElementById('conv-list');
    if (!el) return;
    convSearch.scrollHandler = function() {
      if (convSearch.loading || !convSearch.hasMore) return;
      var threshold = 80; // px from bottom
      if (el.scrollTop + el.clientHeight + threshold >= el.scrollHeight) {
        loadMoreConvSearch();
      }
    };
    el.addEventListener('scroll', convSearch.scrollHandler);
  }

  function detachConvSearchScroll() {
    var el = document.getElementById('conv-list');
    if (el && convSearch.scrollHandler) {
      el.removeEventListener('scroll', convSearch.scrollHandler);
    }
    convSearch.scrollHandler = null;
  }

  async function loadMoreConvSearch() {
    if (convSearch.loading || !convSearch.hasMore) return;
    convSearch.loading = true;
    try {
      var env = await fetchConvSearchPage(convSearch.query, convSearch.offset, convSearch.limit);
      renderConversationSearchEnvelope(env, /*append=*/ true);
    } finally {
      convSearch.loading = false;
    }
  }

  OL.loadConversations = async function() {
    setupConvSearch();
    try {
      var peerGroups = await fetchJSON('/api/conversations/by-peer');
      var el = document.getElementById('conv-list');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No conversations saved yet</div>';
        return;
      }
      var html = '';
      for (var pi = 0; pi < peerGroups.length; pi++) {
        var pg = peerGroups[pi];
        html += '<div class="tree-node peer-header clickable" data-conv-peer="' + pi + '" role="button" tabindex="0" aria-expanded="true">' +
          '<span class="tree-arrow">&#x25BC;</span>' +
          '<span class="status-dot green"></span>' +
          '<span>' + esc(pg.peer_id) + '</span>' +
          '<span class="count">' + pg.count + '</span>' +
        '</div>';
        html += '<div class="peer-children" data-conv-peer-children="' + pi + '">';
        for (var si = 0; si < pg.sessions.length; si++) {
          var sg = pg.sessions[si];
          html += '<div class="tree-node session-header clickable" data-conv-session="' + pi + '-' + si + '" role="button" tabindex="0" aria-expanded="true">' +
            '<span class="tree-arrow badge-sm">&#x25BC;</span>' +
            '<span class="status-dot green dot-sm"></span>' +
            '<span>' + esc(sg.session) + '</span>' +
            '<span class="count">' + sg.count + '</span>' +
          '</div>';
          html += '<div class="session-children" data-conv-session-children="' + pi + '-' + si + '">';
          for (var ci = 0; ci < sg.conversations.length; ci++) {
            var c = sg.conversations[ci];
            html += '<div class="conv-item" data-id="' + esc(c.id) + '" role="button" tabindex="0">' +
              '<div class="conv-item-title">' + esc(c.title) + '</div>' +
              '<div class="conv-item-meta">' +
                '<span>' + c.turn_count + ' turns</span>' +
                '<span>' + formatTime(c.updated_at) + '</span>' +
              '</div>' +
            '</div>';
          }
          html += '</div>';
        }
        html += '</div>';
      }
      el.innerHTML = html;

      // Peer toggle
      OL.wireTreeToggles(el, 'data-conv-peer');

      // Session toggle
      OL.wireTreeToggles(el, 'data-conv-session');

      // Conversation clicks
      el.querySelectorAll('.conv-item').forEach(function(item) {
        var handler = function() {
          el.querySelectorAll('.conv-item').forEach(function(i) { i.classList.remove('active'); });
          item.classList.add('active');
          OL.loadConv(item.dataset.id);
        };
        OL.onView(item, 'click', handler);
        OL.onView(item, 'keydown', function(e) {
          if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handler(); }
        });
      });
    } catch (e) {
      console.error('Load conversations failed:', e);
    }
  };

  // renderConversationSearchItem builds one row. snippet_html is already
  // HTML-escaped on the server with only <mark> tags literal, so it is
  // safe to interpolate directly.
  function renderConversationSearchItem(c) {
    var snippet = c.snippet_html ? '<div class="conv-item-snippet">' + c.snippet_html + '</div>' : '';
    return '<div class="conv-item" data-id="' + esc(c.id) + '" role="button" tabindex="0">' +
      '<div class="conv-item-title">' + esc(c.title) + '</div>' +
      '<div class="conv-item-meta">' +
        '<span class="conv-item-agent">' + esc(c.agent || 'unknown') + '</span>' +
        '<span>' + (c.turn_count || 0) + ' turns</span>' +
      '</div>' +
      snippet +
    '</div>';
  }

  // renderConversationSearchEnvelope handles both initial render and
  // append-on-scroll. Initial render draws keyword results plus an optional
  // "Related by meaning" tail from env.semantic (page 0 only). Append adds
  // only new keyword items.
  function renderConversationSearchEnvelope(env, append) {
    var el = document.getElementById('conv-list');
    if (!el) return;
    var keywordItems = env.items || [];
    var semanticItems = env.semantic || [];
    var total = env.total || 0;

    if (!append) {
      if (!keywordItems.length && !semanticItems.length) {
        el.innerHTML = '<div class="empty-state">No conversations found</div>';
        return;
      }
      var html = '<div class="search-header" style="padding:6px 8px;font-size:11px;color:var(--text-muted)">' +
        esc(String(keywordItems.length)) + ' of ' + esc(String(total)) + ' keyword match' + (total === 1 ? '' : 'es') +
      '</div>';
      html += '<div class="conv-search-keyword-list">' +
        keywordItems.map(renderConversationSearchItem).join('') +
      '</div>';
      if (semanticItems.length) {
        html += '<div class="search-header" style="padding:6px 8px;font-size:11px;color:var(--text-muted);border-top:1px solid var(--border);margin-top:4px">' +
          'Related by meaning (' + esc(String(semanticItems.length)) + ')' +
        '</div>';
        html += '<div class="conv-search-semantic-list">' +
          semanticItems.map(renderConversationSearchItem).join('') +
        '</div>';
      }
      el.innerHTML = html;
    } else {
      var kwList = el.querySelector('.conv-search-keyword-list');
      if (kwList && keywordItems.length) {
        kwList.insertAdjacentHTML('beforeend', keywordItems.map(renderConversationSearchItem).join(''));
      }
      var header = el.querySelector('.search-header');
      if (header) {
        var rendered = el.querySelectorAll('.conv-search-keyword-list .conv-item').length;
        header.textContent = rendered + ' of ' + total + ' keyword match' + (total === 1 ? '' : 'es');
      }
    }

    wireConvItemClicks(el);
  }

  function wireConvItemClicks(el) {
    el.querySelectorAll('.conv-item').forEach(function(item) {
      if (item.dataset.wired === '1') return;
      item.dataset.wired = '1';
      var handler = function() {
        el.querySelectorAll('.conv-item').forEach(function(i) { i.classList.remove('active'); });
        item.classList.add('active');
        OL.loadConv(item.dataset.id);
      };
      OL.onView(item, 'click', handler);
      OL.onView(item, 'keydown', function(e) {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handler(); }
      });
    });
  }

  // Back-compat shim: older call sites may still invoke this with a bare
  // array of conversations. Map to the envelope renderer.
  function renderConversationSearchResults(convos) {
    renderConversationSearchEnvelope({ items: convos || [], total: (convos || []).length }, false);
  }

  // Expose to onclick
  OL.loadConv = async function(id) {
    // Highlight active
    document.querySelectorAll('.conv-item').forEach(function(i) { i.classList.remove('active'); });
    var active = document.querySelector('.conv-item[data-id="' + id + '"]');
    if (active) active.classList.add('active');

    try {
      var conv = await fetchJSON('/api/conversations/' + id);
      renderConversationDetail(conv);
    } catch (e) {
      console.error('Load conversation failed:', e);
    }
  };

  async function renderConversationDetail(conv) {
    var titleEl = document.getElementById('conv-detail-title');
    var bodyEl = document.getElementById('conv-detail');

    var convRef = conv.id ? conv.id.substring(0, 12) : '';
    titleEl.innerHTML = esc(conv.title || 'Conversation') + ' <button class="btn-copy" onclick="OL.copy(\'recall conversation ' + convRef + '\')" title="Copy reference" aria-label="Copy reference">&#x2398;</button>';

    // Fetch linked actions
    var actionsHtml = '';
    try {
      var actions = await fetchJSON('/api/conversations/' + conv.id + '/actions');
      if (actions && actions.length) {
        actionsHtml = '<div class="conv-turn" style="background:var(--bg-card);border-bottom:2px solid var(--border)">' +
          '<div class="conv-turn-header">' +
            '<span class="conv-turn-role" style="color:var(--yellow)">ACTIONS</span>' +
            '<span class="conv-turn-index">' + actions.length + ' tool calls</span>' +
          '</div>' +
          '<div style="display:flex;flex-wrap:wrap;gap:4px;margin-top:4px">' +
            actions.slice(0, 50).map(function(a) {
              return '<span class="badge type action-link" style="cursor:pointer;font-size:10px" data-aid="' + esc(a.id) + '">' + esc(a.tool_name) + '</span>';
            }).join('') +
          '</div>' +
          '<div style="font-size:11px;color:var(--text-muted);margin-top:6px">Click a badge to view the action detail</div>' +
        '</div>';
      }
    } catch (e) {}

    if (!conv.turns || !conv.turns.length) {
      bodyEl.innerHTML = actionsHtml + '<div class="empty-state">No turns in this conversation</div>';
      OL.bindActionLinks(bodyEl);
      return;
    }

    bodyEl.innerHTML = actionsHtml + conv.turns.map(function(t, i) {
      var label = 'You';
      if (t.role === 'assistant') {
        label = formatModel(t.model) || 'Assistant';
      }
      return '<div class="conv-turn ' + esc(t.role) + '">' +
        '<div class="conv-turn-header">' +
          '<span class="conv-turn-role">' + esc(label) + '</span>' +
          '<span class="conv-turn-index">#' + (i + 1) + '</span>' +
          '<button class="btn-copy" onclick="OL.copy(\'recall conversation ' + convRef + ' turn ' + (i + 1) + ': ' + esc(t.content.substring(0, 80)).replace(/'/g, '') + '\')" title="Copy this turn" aria-label="Copy this turn">&#x2398;</button>' +
        '</div>' +
        '<div class="conv-turn-content">' + esc(t.content) + '</div>' +
      '</div>';
    }).join('');

    OL.bindActionLinks(bodyEl);
  }

  OL.bindActionLinks = function(container) {
    container.querySelectorAll('.action-link').forEach(function(el) {
      OL.onView(el, 'click', function() {
        OL.switchView('actions');
        setTimeout(function() { OL.loadActionDetail(el.dataset.aid); }, 300);
      });
    });
  };

  OL.renderConversationSearchResults = renderConversationSearchResults;
})(window.OL);
