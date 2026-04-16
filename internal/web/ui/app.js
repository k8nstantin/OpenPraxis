// OpenLoom — Dashboard entry point
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

  function handleEvent(event) {
    if (event.event === 'stats_update') {
      OL.updateMetrics(event.data);
    } else {
      refreshAll();
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
