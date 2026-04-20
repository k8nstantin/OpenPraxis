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

  // --- Search ---
  function setupSearch() {
    var input = document.getElementById('search-input');
    var btn = document.getElementById('search-btn');
    var close = document.getElementById('search-close');

    btn.addEventListener('click', function() { doSearch(input.value); });
    input.addEventListener('keypress', function(e) {
      if (e.key === 'Enter') doSearch(input.value);
    });
    close.addEventListener('click', function() {
      document.getElementById('search-results').classList.add('hidden');
    });
  }

  async function doSearch(query) {
    if (!query.trim()) return;
    try {
      var resp = await fetch('/api/memories/search', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({query: query, limit: 10})
      });
      var results = await resp.json();
      renderSearchResults(results || []);
    } catch (e) {
      console.error('Search failed:', e);
    }
  }

  function renderSearchResults(results) {
    var body = document.getElementById('search-results-body');
    if (!results.length) {
      body.innerHTML = '<div class="empty-state">No results found</div>';
    } else {
      body.innerHTML = results.map(function(r) {
        return '<div class="search-result-item">' +
          '<span class="search-result-path">' + esc(r.memory.path) + '</span>' +
          '<span class="search-result-score">' + r.score.toFixed(3) + '</span>' +
          '<div class="search-result-content">' + esc(r.memory.l1) + '</div>' +
        '</div>';
      }).join('');
    }
    document.getElementById('search-results').classList.remove('hidden');
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

      var mems = await fetchJSON('/api/memories?prefix=/');
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
