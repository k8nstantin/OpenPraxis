// OpenPraxis — Dashboard entry point
// Loads after api.js, tree.js, and all views/*.js modules.
// Wires navigation, WebSocket, refresh loop, and delegates to OL.* view functions.

(function(OL) {
  'use strict';

  var fetchJSON = OL.fetchJSON, esc = OL.esc, setText = OL.setText;

  // --- State ---
  var ws = null;
  var currentView = 'overview';

  // Expose currentView for other modules (e.g. watcher badge, chat shortcuts)
  OL.currentView = function() { return currentView; };

  // --- Refresh interval (paused when tab hidden) ---
  var refreshInterval = null;

  function startRefreshInterval() {
    if (refreshInterval) return;
    refreshInterval = setInterval(refreshAll, 10000);
  }

  function stopRefreshInterval() {
    if (refreshInterval) {
      clearInterval(refreshInterval);
      refreshInterval = null;
    }
  }

  // Parse a dashboard URL hash. Supported grammar:
  //   #view-<view>
  //   #view-<view>/<arg>
  //   #view-<view>/<arg>/<sub>
  // where <arg> is an entity id (UUID, marker, or date), and <sub> is an
  // optional sub-action like "dag". Returns null if the hash doesn't match.
  function parseHash(hash) {
    if (!hash) return null;
    var m = hash.match(/^#view-([a-z][a-z0-9-]*)(?:\/([^/]+)(?:\/(.+))?)?$/);
    if (!m) return null;
    return { view: m[1], arg: m[2] || '', sub: m[3] || '' };
  }

  // Legacy shim: callers that only care about the view name.
  function viewFromHash(hash) {
    var p = parseHash(hash);
    return p ? p.view : '';
  }

  // Run the view-specific deep-link action for a hash like
  //   #view-<view>/<arg>[/<sub>]
  //
  // arg-only URLs (entity detail):
  //   #view-products/<id>             drill into product + detail pane
  //   #view-manifests/<id>            open the full manifest spec
  //   #view-tasks/<marker>            open task detail (history, actions)
  //   #view-actions/<id>              open action (tool-call) detail
  //   #view-conversations/<id>        open conversation turns
  //   #view-memories/<id>             open memory content
  //   #view-ideas/<id>                open idea detail
  //   #view-watcher/<audit-id>        open a specific watcher audit
  //   #view-cost-history/<yyyy-mm-dd> drill into cost history for a date
  //
  // arg+sub URLs (secondary action on the entity):
  //   #view-products/<id>/dag         render the full-page Cytoscape DAG
  //
  // If a loader or sub-action isn't registered yet (view's JS still
  // initializing), we silently skip — hashchange's setTimeout retry covers
  // the race.
  function applyHashArg(view, arg, sub) {
    if (!arg) return;
    var loader = {
      'products':      OL.loadProductDetail,
      'manifests':     OL.loadManifest,
      'tasks':         OL.loadTaskDetail,
      'actions':       OL.loadActionDetail,
      'conversations': OL.loadConv,
      'memories':      OL.loadMemoryPeerDetail,
      'ideas':         OL.loadIdea,
      'watcher':       OL.loadWatcherDetail,
      'cost-history':  OL.loadCostDrillDown
    }[view];
    if (typeof loader !== 'function') return;
    Promise.resolve(loader(arg)).then(function() {
      if (!sub) return;
      if (view === 'products' && sub === 'dag' && typeof OL.showProductDiagram === 'function') {
        OL.fetchJSON('/api/products/' + encodeURIComponent(arg)).then(function(p) {
          var title = (p && p.title) ? p.title : arg;
          OL.showProductDiagram((p && p.id) ? p.id : arg, title);
        }).catch(function() {
          OL.showProductDiagram(arg, arg);
        });
      }
    });
  }

  // --- Init ---
  document.addEventListener('DOMContentLoaded', function() {
    setupNav();
    setupSearch();
    setupTheme();
    connectWebSocket();
    refreshAll();
    startRefreshInterval();

    document.addEventListener('visibilitychange', function() {
      if (document.hidden) {
        stopRefreshInterval();
      } else {
        refreshAll();
        startRefreshInterval();
      }
    });

    // Handle hash links like #view-ideas
    document.addEventListener('click', function(e) {
      var a = e.target.closest('a[href^="#view-"]');
      if (a) {
        e.preventDefault();
        var view = a.getAttribute('href').replace('#view-', '');
        switchView(view);
      }
    });

    // Deep-link: honor #view-*[/<arg>] from the URL on page load and on
    // hashchange (back/forward nav, external links, headless screenshot
    // tooling). The optional arg drills into a specific entity — for
    // `#view-products/<id>`, that means rendering the product detail + DAG.
    var initial = parseHash(window.location.hash);
    if (initial) {
      switchView(initial.view);
      if (initial.arg) setTimeout(function() { applyHashArg(initial.view, initial.arg, initial.sub); }, 50);
    }
    window.addEventListener('hashchange', function() {
      var p = parseHash(window.location.hash);
      if (!p) return;
      if (p.view !== currentView) switchView(p.view);
      if (p.arg) setTimeout(function() { applyHashArg(p.view, p.arg, p.sub); }, 50);
    });
  });

  // --- Navigation ---
  function setupNav() {
    document.querySelectorAll('.nav-item').forEach(function(item) {
      item.addEventListener('click', function(e) {
        e.preventDefault();
        var view = item.dataset.view;
        switchView(view);
      });
    });
  }

  function switchView(view) {
    var prev = currentView;
    currentView = view;
    OL._switchLifecycle(prev, view);
    document.querySelectorAll('.nav-item').forEach(function(i) { i.classList.remove('active'); });
    var navItem = document.querySelector('[data-view="' + view + '"]');
    if (navItem) navItem.classList.add('active');
    document.querySelectorAll('.view').forEach(function(v) { v.classList.remove('active'); });
    document.getElementById('view-' + view).classList.add('active');
    if (view === 'memories') OL.loadMemoryPeerTree();
    if (view === 'peers') OL.loadPeersView();
    if (view === 'conversations') OL.loadConversations();
    if (view === 'products') OL.loadProducts();
    if (view === 'manifests') OL.loadManifests();
    if (view === 'ideas') OL.loadIdeas();
    if (view === 'tasks') OL.loadTasks();
    // Heavy panels (today's per-task cost rollup, pending/scheduled list)
    // load only when the user lands on Overview or Activity. They used to
    // ship inside the polled stats payload — that froze the dashboard.
    // See idea 019dbb9a-3dc.
    if (view === 'overview' || view === 'activity') {
      if (OL.loadTopTasks) OL.loadTopTasks();
      if (OL.loadPendingTasks) OL.loadPendingTasks();
    }
    if (view === 'productivity') OL.loadProductivityView();
    if (view === 'cost-history') OL.loadCostHistory();
    if (view === 'delusions') OL.loadDelusions();
    if (view === 'watcher') OL.loadWatcher();
    if (view === 'actions') OL.loadActions();
    if (view === 'amnesia') OL.loadAmnesia();
    if (view === 'visceral') OL.loadVisceral();
    if (view === 'activity') OL.renderActivities();
    if (view === 'recall') OL.loadRecall();
    if (view === 'chat') OL.loadChat();
    if (view === 'settings') OL.loadSettings();
    if (view === 'runner') OL.loadRunner();
  }
  OL.switchView = switchView;

  // --- Theme ---
  function setupTheme() {
    var saved = localStorage.getItem('theme') || 'dark';
    document.documentElement.dataset.theme = saved;
    document.getElementById('theme-toggle').addEventListener('click', function() {
      var current = document.documentElement.dataset.theme;
      var next = current === 'dark' ? 'light' : 'dark';
      document.documentElement.dataset.theme = next;
      localStorage.setItem('theme', next);
    });
  }

  // --- Global jump-by-id bar (manifest 019daafb-b5e M4) ---
  // The top-nav #search-input is NOT a catch-all fulltext bar. It's a
  // jump-by-id bar: accepts a UUID or 8–12 char marker, fans out to
  // /api/<type>/search across every entity type, picks the first exact-id
  // hit, and navigates to that entity's detail view. On ambiguous or
  // keyword input, a candidate dropdown is rendered inside the existing
  // #search-results overlay.
  var JUMP_TYPES = [
    { view: 'memories',      endpoint: '/api/memories/search' },
    { view: 'manifests',     endpoint: '/api/manifests/search' },
    { view: 'tasks',         endpoint: '/api/tasks/search' },
    { view: 'products',      endpoint: '/api/products/search' },
    { view: 'ideas',         endpoint: '/api/ideas/search' },
    { view: 'conversations', endpoint: '/api/conversations/search' }
  ];
  var JUMP_DEBOUNCE_MS = 250;

  // Accept UUID, UUID prefix, or 8–12 char hex-with-dashes marker.
  function looksLikeId(s) {
    if (!s) return false;
    return /^[0-9a-f]{8,}(?:-[0-9a-f-]*)?$/i.test(s) && s.length >= 8 && s.length <= 36;
  }

  function candidateFromEntity(view, e) {
    if (!e || typeof e !== 'object') return null;
    var id = e.id || e.ID || '';
    if (!id) return null;
    var marker = e.marker || (id.length >= 12 ? id.slice(0, 12) : id.slice(0, 8));
    var title = e.title || e.Title || e.path || e.summary || e.content || e.l1 || e.tool_name || '';
    return { view: view, id: id, marker: marker, title: String(title).slice(0, 80) };
  }

  // Does a search result contain an exact id/marker match for the query?
  function findExactIdHit(view, results, q) {
    if (!Array.isArray(results)) return null;
    var lq = q.toLowerCase();
    for (var i = 0; i < results.length; i++) {
      var e = results[i];
      var id = (e.id || e.ID || '').toLowerCase();
      var marker = (e.marker || '').toLowerCase();
      if (id === lq || marker === lq || (id && id.indexOf(lq) === 0 && lq.length >= 8)) {
        return candidateFromEntity(view, e);
      }
    }
    return null;
  }

  function setupSearch() {
    var input = document.getElementById('search-input');
    var btn = document.getElementById('search-btn');
    var close = document.getElementById('search-close');
    var timer = null;
    var lastQ = '';

    function trigger() {
      var q = input.value.trim();
      if (!q) {
        hideResults();
        return;
      }
      lastQ = q;
      doJump(q);
    }

    input.addEventListener('input', function() {
      if (timer) { clearTimeout(timer); timer = null; }
      var q = input.value.trim();
      if (!q) { hideResults(); return; }
      timer = setTimeout(function() { timer = null; trigger(); }, JUMP_DEBOUNCE_MS);
    });
    input.addEventListener('keydown', function(e) {
      if (e.key === 'Enter') {
        e.preventDefault();
        if (timer) { clearTimeout(timer); timer = null; }
        trigger();
      } else if (e.key === 'Escape') {
        input.value = '';
        hideResults();
      }
    });
    btn.addEventListener('click', function() {
      if (timer) { clearTimeout(timer); timer = null; }
      trigger();
    });
    close.addEventListener('click', hideResults);

    // Expose for diagnostics
    OL._jumpCurrentQuery = function() { return lastQ; };
  }

  function hideResults() {
    document.getElementById('search-results').classList.add('hidden');
  }

  async function doJump(query) {
    var isId = looksLikeId(query);
    var url = '?q=' + encodeURIComponent(query) + '&limit=10';
    var fetches = JUMP_TYPES.map(function(t) {
      return fetch(t.endpoint + url).then(function(r) {
        return r.ok ? r.json() : [];
      }).then(function(list) {
        return { view: t.view, results: Array.isArray(list) ? list : (list && list.items) || [] };
      }).catch(function() {
        return { view: t.view, results: [] };
      });
    });

    var all;
    try {
      all = await Promise.all(fetches);
    } catch (e) {
      console.error('Jump failed:', e);
      return;
    }
    // Stale check — user kept typing past this fetch.
    if (query !== document.getElementById('search-input').value.trim()) return;

    // 1. Look for exact id/marker hit across all types.
    if (isId) {
      for (var i = 0; i < all.length; i++) {
        var hit = findExactIdHit(all[i].view, all[i].results, query);
        if (hit) {
          jumpTo(hit);
          return;
        }
      }
    }

    // 2. Build candidate list (up to 5 per type).
    var candidates = [];
    all.forEach(function(bucket) {
      (bucket.results || []).slice(0, 5).forEach(function(e) {
        var c = candidateFromEntity(bucket.view, e);
        if (c) candidates.push(c);
      });
    });

    if (!candidates.length) {
      renderNoMatch(query);
      return;
    }
    renderCandidates(query, candidates, !isId);
  }

  function jumpTo(cand) {
    var input = document.getElementById('search-input');
    input.value = '';
    hideResults();
    location.hash = '#view-' + cand.view + '/' + encodeURIComponent(cand.id);
  }

  function renderNoMatch(q) {
    var body = document.getElementById('search-results-body');
    body.innerHTML = '<div class="empty-state">No match for ' +
      '<code style="font-family:var(--font-mono)">' + esc(q) + '</code></div>';
    document.getElementById('search-results').classList.remove('hidden');
  }

  function renderCandidates(q, candidates, isKeyword) {
    var body = document.getElementById('search-results-body');
    var hint = isKeyword
      ? '<div class="empty-state" style="padding-bottom:8px">' +
        'No exact id match. For full-text search use the tab\'s search bar.</div>'
      : '';
    var rows = candidates.map(function(c, idx) {
      return '<div class="search-result-item jump-candidate" data-idx="' + idx + '" ' +
        'style="cursor:pointer;display:flex;gap:10px;align-items:baseline">' +
        '<span class="search-result-score" style="text-transform:uppercase;min-width:80px">' +
          esc(c.view) + '</span>' +
        '<span class="search-result-path">' + esc(c.marker) + '</span>' +
        '<span class="search-result-content" style="flex:1">' + esc(c.title || '(untitled)') + '</span>' +
      '</div>';
    }).join('');
    body.innerHTML = hint + rows;
    document.getElementById('search-results').classList.remove('hidden');

    body.querySelectorAll('.jump-candidate').forEach(function(el) {
      el.addEventListener('click', function() {
        var idx = parseInt(el.dataset.idx, 10);
        if (!isNaN(idx) && candidates[idx]) jumpTo(candidates[idx]);
      });
    });
  }

  // --- WebSocket ---
  function connectWebSocket() {
    var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(proto + '//' + location.host + '/ws');

    ws.onmessage = function(e) {
      try {
        var event = JSON.parse(e.data);
        handleEvent(event);
      } catch (err) {}
    };

    ws.onclose = function() {
      setTimeout(connectWebSocket, 3000);
    };
  }

  // Debounced refresh — collapse bursts of WebSocket events into one fetch
  // cascade. A running agent can fire 20-50 events in quick succession (each
  // tool call + hook + cost record); pre-debounce every event triggered 10
  // sequential fetchJSON calls in refreshAll(), drowning the browser. 500ms
  // trailing-edge is imperceptible to the user and caps refresh rate at 2Hz.
  var refreshDebounceTimer = null;
  function refreshAllDebounced() {
    if (refreshDebounceTimer) return;
    refreshDebounceTimer = setTimeout(function() {
      refreshDebounceTimer = null;
      refreshAll();
    }, 500);
  }

  function handleEvent(event) {
    if (event.event === 'stats_update') {
      OL.updateMetrics(event.data);
    } else {
      refreshAllDebounced();
    }
  }

  // --- Data Loading ---
  async function refreshAll() {
    try {
      var status = await fetchJSON('/api/status');
      OL.updateMetrics(status);
      var displayName = status.display_name || status.node || 'unknown';
      document.getElementById('node-name').textContent = displayName;
      var headerUuid = document.getElementById('header-uuid');
      if (headerUuid) {
        var nodeId = status.node || '';
        headerUuid.textContent = nodeId.length > 12 ? nodeId.substring(0, 12) : nodeId;
        headerUuid.title = nodeId;
      }
      var headerAvatar = document.getElementById('header-avatar');
      if (status.avatar) {
        if (status.avatar.startsWith('http')) {
          headerAvatar.innerHTML = '<img src="' + status.avatar + '" style="width:24px;height:24px;border-radius:50%;object-fit:cover;vertical-align:middle" />';
        } else {
          headerAvatar.textContent = status.avatar;
        }
        var sidebarAvatar = document.getElementById('sidebar-avatar');
        if (sidebarAvatar && !status.avatar.startsWith('http')) {
          sidebarAvatar.textContent = status.avatar;
        }
      }

      var nodes = await fetchJSON('/api/peers');
      OL.renderPeers(nodes || []);
      if (currentView === 'peers') OL.renderPeersList(nodes || []);

      var mems = await fetchJSON('/api/memories?prefix=/&limit=10&summary=true');
      OL.renderRecentMemories(mems || []);

      var markers = await fetchJSON('/api/markers?status=pending');
      OL.renderMarkers(markers || []);

      if (currentView === 'conversations') OL.loadConversations();
      OL.updateAmnesiaCount();
      OL.updateDelusionCount();
      OL.loadRunningTasks();
      OL.loadTaskStats();
      OL.updateProductivity();
    } catch (e) {
      console.error('Refresh failed:', e);
    }
  }
  OL.refreshAll = refreshAll;

})(window.OL);
