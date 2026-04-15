// OpenLoom — Dashboard

(function() {
  'use strict';

  // --- State ---
  let ws = null;
  let currentView = 'overview';

  // --- Init ---
  document.addEventListener('DOMContentLoaded', () => {
    setupNav();
    setupSearch();
    setupTheme();
    connectWebSocket();
    refreshAll();
    setInterval(refreshAll, 10000);

    // Handle hash links like #view-ideas
    document.addEventListener('click', (e) => {
      const a = e.target.closest('a[href^="#view-"]');
      if (a) {
        e.preventDefault();
        const view = a.getAttribute('href').replace('#view-', '');
        switchView(view);
      }
    });
  });

  // --- Navigation ---
  function setupNav() {
    document.querySelectorAll('.nav-item').forEach(item => {
      item.addEventListener('click', (e) => {
        e.preventDefault();
        const view = item.dataset.view;
        switchView(view);
      });
    });
  }

  window.switchView = function switchView(view) {
    currentView = view;
    document.querySelectorAll('.nav-item').forEach(i => i.classList.remove('active'));
    const navItem = document.querySelector(`[data-view="${view}"]`);
    if (navItem) navItem.classList.add('active');
    document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
    document.getElementById(`view-${view}`).classList.add('active');
    if (view === 'memories') loadMemoryPeerTree();
    if (view === 'peers') loadPeersView();
    if (view === 'conversations') loadConversations();
    if (view === 'products') loadProducts();
    if (view === 'manifests') loadManifests();
    if (view === 'ideas') loadIdeas();
    if (view === 'tasks') loadTasks();
    if (view === 'cost-history') loadCostHistory();
    if (view === 'delusions') loadDelusions();
    if (view === 'watcher') loadWatcher();
    if (view === 'actions') loadActions();
    if (view === 'amnesia') loadAmnesia();
    if (view === 'visceral') loadVisceral();
    if (view === 'activity') renderActivities();
    if (view === 'recall') loadRecall();
    if (view === 'chat') loadChat();
    if (view === 'settings') loadSettings();
  }

  // --- Theme ---
  function setupTheme() {
    const saved = localStorage.getItem('theme') || 'dark';
    document.documentElement.dataset.theme = saved;
    document.getElementById('theme-toggle').addEventListener('click', () => {
      const current = document.documentElement.dataset.theme;
      const next = current === 'dark' ? 'light' : 'dark';
      document.documentElement.dataset.theme = next;
      localStorage.setItem('theme', next);
    });
  }

  // --- Search ---
  function setupSearch() {
    const input = document.getElementById('search-input');
    const btn = document.getElementById('search-btn');
    const close = document.getElementById('search-close');

    btn.addEventListener('click', () => doSearch(input.value));
    input.addEventListener('keypress', (e) => {
      if (e.key === 'Enter') doSearch(input.value);
    });
    close.addEventListener('click', () => {
      document.getElementById('search-results').classList.add('hidden');
    });
  }

  async function doSearch(query) {
    if (!query.trim()) return;
    try {
      const resp = await fetch('/api/memories/search', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({query, limit: 10})
      });
      const results = await resp.json();
      renderSearchResults(results || []);
    } catch (e) {
      console.error('Search failed:', e);
    }
  }

  function renderSearchResults(results) {
    const body = document.getElementById('search-results-body');
    if (!results.length) {
      body.innerHTML = '<div class="empty-state">No results found</div>';
    } else {
      body.innerHTML = results.map(r => `
        <div class="search-result-item">
          <span class="search-result-path">${esc(r.memory.path)}</span>
          <span class="search-result-score">${r.score.toFixed(3)}</span>
          <div class="search-result-content">${esc(r.memory.l1)}</div>
        </div>
      `).join('');
    }
    document.getElementById('search-results').classList.remove('hidden');
  }

  // --- WebSocket ---
  function connectWebSocket() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${proto}//${location.host}/ws`);

    ws.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data);
        handleEvent(event);
      } catch (err) {}
    };

    ws.onclose = () => {
      setTimeout(connectWebSocket, 3000);
    };
  }

  function handleEvent(event) {
    if (event.event === 'stats_update') {
      updateMetrics(event.data);
    } else {
      refreshAll();
    }
  }

  // --- Data Loading ---
  async function refreshAll() {
    try {
      const status = await fetchJSON('/api/status');
      updateMetrics(status);
      const displayName = status.display_name || status.node || 'unknown';
      document.getElementById('node-name').textContent = displayName;
      const headerUuid = document.getElementById('header-uuid');
      if (headerUuid) {
        const nodeId = status.node || '';
        headerUuid.textContent = nodeId.length > 12 ? nodeId.substring(0, 12) : nodeId;
        headerUuid.title = nodeId;
      }
      const headerAvatar = document.getElementById('header-avatar');
      if (status.avatar) {
        if (status.avatar.startsWith('http')) {
          headerAvatar.innerHTML = `<img src="${status.avatar}" style="width:24px;height:24px;border-radius:50%;object-fit:cover;vertical-align:middle" />`;
        } else {
          headerAvatar.textContent = status.avatar;
        }
        const sidebarAvatar = document.getElementById('sidebar-avatar');
        if (sidebarAvatar && !status.avatar.startsWith('http')) {
          sidebarAvatar.textContent = status.avatar;
        }
      }

      const nodes = await fetchJSON('/api/peers');
      renderPeers(nodes || []);
      if (currentView === 'peers') renderPeersList(nodes || []);

      const mems = await fetchJSON('/api/memories?prefix=/');
      renderRecentMemories(mems || []);

      const markers = await fetchJSON('/api/markers?status=pending');
      renderMarkers(markers || []);

      if (currentView === 'conversations') loadConversations();
      updateAmnesiaCount();
      updateDelusionCount();
      loadRunningTasks();
      loadTaskStats();
    } catch (e) {
      console.error('Refresh failed:', e);
    }
  }

  async function loadTaskStats() {
    try {
      const stats = await fetchJSON('/api/tasks/stats');
      if (stats) updateTaskStats(stats);
    } catch (e) {}
  }

  async function loadRunningTasks() {
    try {
      const tasks = await fetchJSON('/api/tasks/running');
      const hasRunning = tasks && tasks.length > 0;

      // Render to both overview and activity panels
      const targets = [
        {panel: 'running-tasks-panel', list: 'running-tasks-list', badge: 'running-tasks-count'},
        {panel: 'activity-running-panel', list: 'activity-running-list', badge: null},
        {panel: 'tasks-running-panel', list: 'tasks-running-list', badge: null},
      ];

      for (const t of targets) {
        const panel = document.getElementById(t.panel);
        const el = document.getElementById(t.list);
        if (!panel || !el) continue;

        if (!hasRunning) {
          panel.style.display = 'none';
          continue;
        }

        panel.style.display = '';
        if (t.badge) {
          const badge = document.getElementById(t.badge);
          if (badge) badge.textContent = tasks.length;
        }

        el.innerHTML = tasks.map(rt => {
          const elapsed = Math.round((Date.now() - new Date(rt.started_at).getTime()) / 1000);
          const mins = Math.floor(elapsed / 60);
          const secs = elapsed % 60;
          return `<div class="peer-row clickable running-task-row" data-task-id="${esc(rt.task_id)}" style="flex-wrap:wrap;cursor:pointer">
            <span class="status-dot ${rt.paused ? 'yellow' : 'green'}" style="${rt.paused ? '' : 'animation:pulse 1s infinite'}"></span>
            <span class="session-uuid">${esc(rt.marker)}</span>
            <span style="font-weight:500;font-size:13px;flex:1">${esc(rt.title)}</span>
            <span class="badge type" style="font-size:10px">${esc(rt.agent)}</span>
            <span style="font-size:12px;color:var(--text-muted)">${rt.actions} actions</span>
            <span style="font-size:12px;color:${rt.paused ? 'var(--yellow)' : 'var(--green)'};font-weight:500">${rt.paused ? 'PAUSED' : mins+'m '+secs+'s'}</span>
            ${rt.paused
              ? `<button class="btn-search" onclick="event.stopPropagation();window._resumeTask('${esc(rt.task_id)}')" style="font-size:11px;padding:3px 10px">Resume</button>`
              : `<button class="btn-search" onclick="event.stopPropagation();window._pauseTask('${esc(rt.task_id)}')" style="font-size:11px;padding:3px 10px;background:var(--yellow);color:var(--bg-primary)">Pause</button>`}
            <button class="btn-confirm" onclick="event.stopPropagation();window._killTask('${esc(rt.task_id)}')" style="font-size:11px;padding:3px 10px">Stop</button>
          </div>`;
        }).join('');

        // Click row → go to Tasks tab → open task detail with live output
        el.querySelectorAll('.running-task-row').forEach(row => {
          row.addEventListener('click', () => {
            switchView('tasks');
            setTimeout(() => loadTaskDetail(row.dataset.taskId), 300);
          });
        });
      }
    } catch (e) {}
  }

  window._killTask = async function(id) {
    if (!confirm('Stop this running task?')) return;
    await fetch('/api/tasks/' + id + '/kill', {method: 'POST'});
    loadRunningTasks();
    loadTaskDetail(id);
    loadTasks();
  };

  window._pauseTask = async function(id) {
    await fetch('/api/tasks/' + id + '/pause', {method: 'POST'});
    loadRunningTasks();
    loadTaskDetail(id);
    loadTasks();
  };

  window._resumeTask = async function(id) {
    await fetch('/api/tasks/' + id + '/resume', {method: 'POST'});
    loadRunningTasks();
    loadTaskDetail(id);
    loadTasks();
  };

  window._emergencyStopAll = async function() {
    if (!confirm('EMERGENCY STOP — Kill ALL running tasks?')) return;
    try {
      const tasks = await fetchJSON('/api/tasks/running');
      if (tasks) {
        for (const t of tasks) {
          await fetch('/api/tasks/' + t.task_id + '/kill', {method: 'POST'});
        }
      }
      loadRunningTasks();
    } catch (e) {}
  };

  // Wire emergency stop buttons
  document.addEventListener('DOMContentLoaded', () => {
    const btn1 = document.getElementById('emergency-stop-btn');
    const btn2 = document.getElementById('activity-emergency-stop');
    if (btn1) btn1.onclick = window._emergencyStopAll;
    if (btn2) btn2.onclick = window._emergencyStopAll;
  });

  function updateMetrics(data) {
    setText('metric-memories', data.memories ?? '-');
    setText('metric-peers', data.peers ?? '-');
    setText('metric-agents', data.sessions ?? data.agents ?? '-');
    setText('metric-uptime', data.uptime || '-');
    setText('sidebar-peers', data.peers ?? 0);
    setText('sidebar-agents', data.sessions ?? data.agents ?? 0);
    const markerCount = data.markers ?? 0;
    setText('sidebar-markers', markerCount);
    const markerEl = document.getElementById('sidebar-markers');
    if (markerEl) markerEl.classList.toggle('has-markers', markerCount > 0);
  }

  function updateTaskStats(stats) {
    // Row 1: running, turns, cost, tasks
    const runningCount = stats.running ?? 0;
    setText('metric-running', runningCount);
    const runningCard = document.getElementById('metric-running-card');
    if (runningCard) {
      const valEl = runningCard.querySelector('.metric-value');
      if (valEl) valEl.style.color = runningCount > 0 ? 'var(--green)' : 'var(--text-muted)';
      runningCard.style.borderColor = runningCount > 0 ? 'var(--green)' : 'var(--border)';
    }

    setText('metric-turns-today', stats.turns_today ?? 0);
    const cost = stats.cost_today ?? 0;
    const budget = stats.daily_budget ?? 0;
    const budgetPct = stats.budget_pct ?? 0;

    // Show "$X / $Y" format when budget is set, otherwise just "$X"
    const costValEl = document.getElementById('metric-cost-today');
    if (budget > 0) {
      setText('metric-cost-today', '$' + cost.toFixed(2) + ' / $' + budget.toFixed(0));
      if (costValEl) costValEl.style.fontSize = '22px';
    } else {
      setText('metric-cost-today', '$' + cost.toFixed(2));
      if (costValEl) costValEl.style.fontSize = '';
    }

    // Color logic: green < 80%, yellow 80-100%, red > 100% (with pulse)
    const costEl = document.getElementById('metric-cost-today');
    const costCard = document.getElementById('metric-cost-card');
    if (costEl) {
      if (budget > 0) {
        if (budgetPct >= 100) {
          costEl.style.color = 'var(--red)';
          costEl.style.animation = 'pulse 1s infinite';
          if (costCard) costCard.style.borderColor = 'var(--red)';
        } else if (budgetPct >= 80) {
          costEl.style.color = 'var(--yellow)';
          costEl.style.animation = '';
          if (costCard) costCard.style.borderColor = 'var(--yellow)';
        } else {
          costEl.style.color = 'var(--green)';
          costEl.style.animation = '';
          if (costCard) costCard.style.borderColor = '';
        }
      } else {
        // No budget set — always green
        costEl.style.color = 'var(--green)';
        costEl.style.animation = '';
        if (costCard) costCard.style.borderColor = '';
      }
    }
    setText('metric-tasks-total', stats.tasks_total ?? 0);

    // Top tasks panel
    const panel = document.getElementById('top-tasks-panel');
    const list = document.getElementById('top-tasks-list');
    const topTasks = stats.top_tasks || [];
    if (!topTasks.length) {
      if (panel) panel.style.display = 'none';
      return;
    }
    if (panel) panel.style.display = '';

    const statusColors = {running:'var(--green)',paused:'var(--yellow)',scheduled:'var(--yellow)',waiting:'var(--accent)',pending:'var(--text-muted)',completed:'var(--green)',max_turns:'var(--yellow)',failed:'var(--red)',cancelled:'var(--text-muted)'};

    let totalCost = topTasks.reduce((s, t) => s + t.cost, 0);
    let html = `<div style="max-height:300px;overflow-y:auto"><table class="top-tasks-table" style="width:100%">
      <thead style="position:sticky;top:0;background:var(--bg-primary);z-index:1"><tr>
        <th style="text-align:left;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">marker</th>
        <th style="text-align:left;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">title</th>
        <th style="text-align:left;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">branch</th>
        <th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">turns</th>
        <th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">cost</th>
        <th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">status</th>
      </tr></thead><tbody>`;

    for (const t of topTasks) {
      const costColor = t.cost > 5 ? 'var(--red)' : t.cost > 1 ? 'var(--yellow)' : 'var(--green)';
      const sColor = statusColors[t.status] || 'var(--text-muted)';
      const titleTrunc = t.title.length > 40 ? t.title.substring(0, 40) + '...' : t.title;
      html += `<tr class="top-task-row clickable" onclick="switchView('tasks');setTimeout(()=>loadTaskDetail&&loadTaskDetail('${esc(t.marker)}'),200)">
        <td style="padding:6px 12px;font-family:var(--font-mono);font-size:11px;color:var(--accent)">${esc(t.marker)}</td>
        <td style="padding:6px 12px;font-size:12px">${esc(titleTrunc)}</td>
        <td style="padding:6px 12px;font-family:var(--font-mono);font-size:10px;color:var(--text-muted)">openloom/${esc(t.marker)}</td>
        <td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">${t.turns}</td>
        <td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;color:${costColor}">$${t.cost.toFixed(2)}</td>
        <td style="padding:6px 12px;text-align:right;font-size:11px;color:${sColor}">${esc(t.status)}</td>
      </tr>`;
    }
    html += `</tbody><tfoot><tr style="border-top:2px solid var(--border)">
      <td colspan="4" style="padding:6px 12px;font-size:12px;font-weight:600">Total (${topTasks.length} tasks)</td>
      <td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600;color:var(--green)">$${totalCost.toFixed(2)}</td>
      <td></td>
    </tr></tfoot></table></div>`;
    if (list) list.innerHTML = html;

    // Pending/scheduled tasks panel
    const pendingPanel = document.getElementById('pending-tasks-panel');
    const pendingList = document.getElementById('pending-tasks-list');
    const pending = stats.pending_tasks || [];
    if (!pending.length) {
      if (pendingPanel) pendingPanel.style.display = 'none';
    } else {
      if (pendingPanel) pendingPanel.style.display = '';
      const statusColors = {scheduled:'var(--yellow)',waiting:'var(--accent)',pending:'var(--text-muted)'};
      const statusIcons = {scheduled:'&#x23F0;',waiting:'&#x23F3;',pending:'&#x25CB;'};
      let phtml = '';
      for (const t of pending) {
        const sc = statusColors[t.status] || 'var(--text-muted)';
        let when = '';
        if (t.next_run_at) {
          const diff = Math.round((new Date(t.next_run_at) - Date.now()) / 60000);
          when = diff > 0 ? `in ${diff}m` : 'due';
        }
        if (t.depends_on) when = `after ${t.depends_on}`;
        phtml += `<div class="peer-row clickable" onclick="switchView('tasks')" style="padding:6px 0">
          <span style="color:${sc};font-size:12px">${statusIcons[t.status] || ''}</span>
          <span class="session-uuid">${esc(t.marker)}</span>
          <span style="font-size:12px;flex:1">${esc(t.title.length > 40 ? t.title.substring(0,40) + '...' : t.title)}</span>
          <span style="font-size:11px;color:${sc};font-weight:500">${when}</span>
        </div>`;
      }
      if (pendingList) pendingList.innerHTML = phtml;
    }
  }

  function renderPeers(nodes) {
    const el = document.getElementById('overview-peers');
    if (!nodes.length) {
      el.innerHTML = '<div class="empty-state">No nodes</div>';
      return;
    }
    el.innerHTML = nodes.map(n => {
      const badge = n.is_local ? '<span class="badge scope">local</span>' : '<span class="badge tag">remote</span>';
      const sessions = (n.sessions || []).map(s => `
        <div class="agent-row" style="padding-left:20px">
          <span class="status-dot green"></span>
          <span class="agent-name">${esc(s.agent)}</span>
          <span class="session-uuid">${esc(s.uuid ? s.uuid.substring(0, 12) : '')}</span>
          <span class="agent-meta">${s.turn_count ? s.turn_count + ' turns' : (s.tool_calls || 0) + ' calls'} &middot; ${timeAgo(s.connected_at)}</span>
        </div>
      `).join('');
      const sessionCount = (n.sessions || []).length;
      return `<div class="node-group">
        <div class="node-group-header">
          <span class="status-dot ${n.status === 'online' ? 'green' : 'yellow'}"></span>
          ${esc(n.node_id)} ${badge}
          <span style="margin-left:auto;font-size:11px;color:var(--text-muted)">${n.memories || 0} memories &middot; ${sessionCount} session${sessionCount !== 1 ? 's' : ''}</span>
        </div>
        ${sessions || '<div style="padding:4px 20px;font-size:12px;color:var(--text-muted)">No active sessions</div>'}
      </div>`;
    }).join('');
  }

  function renderPeersList(nodes) {
    const el = document.getElementById('peers-list');
    if (!nodes.length) {
      el.innerHTML = '<div class="empty-state">No nodes</div>';
      return;
    }
    el.innerHTML = nodes.map(n => {
      const badge = n.is_local ? '<span class="badge scope">local</span>' : '<span class="badge tag">remote</span>';
      const sessions = (n.sessions || []).map(s => `
        <div class="agent-row" style="padding-left:20px">
          <span class="status-dot green"></span>
          <span class="agent-name">${esc(s.agent)}</span>
          <span class="session-uuid">${esc(s.uuid ? s.uuid.substring(0, 12) : '')}</span>
          <span class="agent-meta">${s.turn_count ? s.turn_count + ' turns' : (s.tool_calls || 0) + ' calls'} &middot; ${timeAgo(s.connected_at)}</span>
        </div>
      `).join('');
      const sessionCount = (n.sessions || []).length;
      return `<div class="node-group" style="margin-bottom:16px">
        <div class="node-group-header" style="font-size:14px">
          <span class="status-dot ${n.status === 'online' ? 'green' : 'yellow'}"></span>
          ${esc(n.node_id)} ${badge}
          <span style="margin-left:auto;font-size:12px;color:var(--text-muted)">${n.memories || 0} memories</span>
        </div>
        ${sessions || '<div style="padding:8px 20px;font-size:13px;color:var(--text-muted)">No active sessions</div>'}
      </div>`;
    }).join('');
  }

  async function loadPeersView() {
    try {
      const nodes = await fetchJSON('/api/peers');
      renderPeersList(nodes || []);
    } catch (e) {
      console.error('Load peers failed:', e);
    }
  }

  function renderRecentMemories(mems) {
    const el = document.getElementById('overview-memories');
    if (!mems.length) {
      el.innerHTML = '<div class="empty-state">No memories stored yet</div>';
      return;
    }
    el.innerHTML = mems.slice(0, 10).map(m => {
      const marker = m.id ? m.id.substring(0, 12) : '';
      const session = m.source_agent || '';
      return `
      <div class="memory-row clickable" data-memory-id="${esc(m.id)}">
        <span class="session-uuid">${esc(marker)}</span>
        <span class="badge type">${esc(m.type)}</span>
        <span style="color:var(--text-primary);font-size:13px">${esc(m.l0)}</span>
        <span style="color:var(--text-muted);font-size:11px;margin-left:auto">${esc(session)}</span>
      </div>`;
    }).join('');
    el.querySelectorAll('.memory-row').forEach(row => {
      row.addEventListener('click', () => {
        switchView('memories');
        loadMemoryPeerDetail(row.dataset.memoryId);
      });
    });
  }

  // --- Memories by Peer ---
  async function loadMemoryPeerTree() {
    try {
      const peerGroups = await fetchJSON('/api/memories/by-peer');
      const el = document.getElementById('memory-peer-tree');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No peers</div>';
        return;
      }
      let html = '';
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];
        html += `<div class="tree-node peer-header clickable" data-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-peer-children="${pi}">`;
        for (let si = 0; si < pg.sessions.length; si++) {
          const sg = pg.sessions[si];
          html += `<div class="tree-node session-header clickable" data-session="${pi}-${si}">
            <span class="tree-arrow" style="font-size:10px">&#x25BC;</span>
            <span class="status-dot green" style="width:6px;height:6px"></span>
            <span>${esc(sg.session)}</span>
            <span class="count">${sg.count}</span>
          </div>`;
          html += `<div class="session-children" data-session-children="${pi}-${si}">`;
          for (const m of sg.memories) {
            html += `<div class="tree-node peer-leaf clickable" data-memory-id="${esc(m.id)}">
              <span class="session-uuid">${esc(m.marker)}</span>
              <span style="font-size:12px;color:var(--text-primary);flex:1">${esc(m.l0.length > 50 ? m.l0.substring(0,50) + '...' : m.l0)}</span>
              <button class="btn-copy-sm" onclick="event.stopPropagation();window._copy('recall memory ${esc(m.marker)}')" title="Copy ref">&#x2398;</button>
            </div>`;
          }
          html += `</div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer header toggle
      el.querySelectorAll('.tree-node.peer-header').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.peer;
          const children = el.querySelector(`[data-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Session header toggle
      el.querySelectorAll('.tree-node.session-header').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.session;
          const children = el.querySelector(`[data-session-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Memory leaf clicks
      el.querySelectorAll('.tree-node.peer-leaf').forEach(node => {
        node.addEventListener('click', () => {
          el.querySelectorAll('.tree-node').forEach(n => n.classList.remove('active'));
          node.classList.add('active');
          loadMemoryPeerDetail(node.dataset.memoryId);
        });
      });
    } catch (e) {
      console.error('Load peer memories failed:', e);
    }
  }

  async function loadMemoryPeerDetail(id) {
    try {
      const mem = await fetchJSON('/api/memories/' + id);
      if (mem) renderMemoryPeerDetail(mem);
    } catch (e) {
      console.error('Load memory peer detail failed:', e);
    }
  }

  function renderMemoryPeerDetail(mem) {
    const detail = document.getElementById('memory-peer-detail');
    let currentTier = 'l2';
    const tierContent = { l0: mem.l0 || '', l1: mem.l1 || '', l2: mem.l2 || '' };
    const marker = mem.id ? mem.id.substring(0, 12) : '';
    const session = mem.source_agent || 'unknown';
    const node = mem.source_node || 'unknown';

    detail.innerHTML = `
      <div class="memory-detail-view">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px">
          <span class="session-uuid" style="font-size:14px">${esc(marker)}</span>
          <span style="font-family:var(--font-mono);font-size:12px;color:var(--accent);flex:1">${esc(mem.path)}</span>
          <button class="btn-copy" onclick="window._copy('recall memory ${marker}')" title="Copy reference">&#x2398;</button>
        </div>
        <div class="memory-meta">
          <span class="badge type">${esc(mem.type || 'insight')}</span>
          <span class="badge scope">${esc(mem.scope || 'project')}</span>
          ${mem.project ? `<span class="badge">${esc(mem.project)}</span>` : ''}
          ${mem.domain ? `<span class="badge">${esc(mem.domain)}</span>` : ''}
          <span class="badge" style="color:var(--green)">${esc(session)}</span>
          <span class="badge" style="color:var(--accent)">${esc(node)}</span>
        </div>
        <div class="tier-tabs">
          <div class="tier-tab" data-tier="l0">L0 — One-liner</div>
          <div class="tier-tab" data-tier="l1">L1 — Summary</div>
          <div class="tier-tab active" data-tier="l2">L2 — Full</div>
        </div>
        <div class="memory-content" id="memory-peer-content-text">${esc(tierContent.l2)}</div>
        <div class="memory-timestamps" style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border);font-size:11px;color:var(--text-muted);display:flex;gap:16px;flex-wrap:wrap">
          <span>Created: ${mem.created_at ? new Date(mem.created_at).toLocaleString() : '-'}</span>
          <span>Updated: ${mem.updated_at ? new Date(mem.updated_at).toLocaleString() : '-'}</span>
          <span>Accessed: ${mem.access_count || 0} times</span>
          <span>Peer: ${esc(node)}</span>
          <span>Session: ${esc(session)}</span>
        </div>
        <div style="margin-top:8px;font-family:var(--font-mono);font-size:10px;color:var(--text-muted)">ID: ${esc(mem.id)}</div>
        <div style="margin-top:12px;display:flex;gap:8px">
          <button class="btn-search" id="mem-promote-btn" style="font-size:12px;padding:6px 14px">Create Manifest from Memory</button>
          <button class="btn-dismiss" onclick="window._archiveMemory('${esc(mem.id)}')" style="font-size:12px;padding:6px 14px">Archive</button>
        </div>
      </div>
    `;

    detail.querySelector('#mem-promote-btn').addEventListener('click', () => {
      window._promoteToManifest(
        tierContent.l0,
        tierContent.l1,
        tierContent.l2
      );
    });

    detail.querySelectorAll('.tier-tab').forEach(tab => {
      tab.addEventListener('click', () => {
        detail.querySelectorAll('.tier-tab').forEach(t => t.classList.remove('active'));
        tab.classList.add('active');
        document.getElementById('memory-peer-content-text').textContent = tierContent[tab.dataset.tier];
      });
    });
  }

  // --- Activity ---
  async function renderActivities() {
    const el = document.getElementById('activity-feed');
    try {
      const peerGroups = await fetchJSON('/api/activity/by-peer');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No activity yet</div>';
        return;
      }
      let html = '';
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];
        html += `<div class="tree-node peer-header clickable" data-act-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-act-peer-children="${pi}">`;
        for (let si = 0; si < pg.sessions.length; si++) {
          const sg = pg.sessions[si];
          html += `<div class="tree-node session-header clickable" data-act-session="${pi}-${si}">
            <span class="tree-arrow" style="font-size:10px">&#x25BC;</span>
            <span class="status-dot green" style="width:6px;height:6px"></span>
            <span>${esc(sg.session)}</span>
            <span class="count">${sg.count}</span>
          </div>`;
          html += `<div class="session-children" data-act-session-children="${pi}-${si}">`;
          for (const a of sg.activities) {
            const icon = a.type === 'memory' ? '&#x25CF;' : '&#x2631;';
            const badge = a.type === 'memory'
              ? '<span class="badge type" style="font-size:10px">memory</span>'
              : '<span class="badge scope" style="font-size:10px">conversation</span>';
            html += `<div class="activity-item clickable" data-activity-type="${esc(a.type)}" data-activity-id="${esc(a.id)}">
              <span class="activity-time">${formatTime(a.time)}</span>
              <span class="activity-icon">${icon}</span>
              ${badge}
              <span class="activity-text">${esc(a.title)}</span>
            </div>`;
          }
          html += `</div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer toggle
      el.querySelectorAll('[data-act-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.actPeer;
          const children = el.querySelector(`[data-act-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Session toggle
      el.querySelectorAll('[data-act-session]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.actSession;
          const children = el.querySelector(`[data-act-session-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Activity item click — cross-navigate
      el.querySelectorAll('.activity-item').forEach(item => {
        item.addEventListener('click', () => {
          const type = item.dataset.activityType;
          const id = item.dataset.activityId;
          if (type === 'conversation') {
            switchView('conversations');
            setTimeout(() => window._loadConv(id), 300);
          } else if (type === 'memory') {
            switchView('memories');
            setTimeout(() => loadMemoryPeerDetail(id), 300);
          }
        });
      });
    } catch (e) {
      console.error('Load activity failed:', e);
    }
  }

  // --- Markers ---
  function renderMarkers(markers) {
    const panel = document.getElementById('markers-panel');
    const el = document.getElementById('overview-markers');
    const badge = document.getElementById('marker-count-badge');

    if (!markers.length) {
      if (panel) panel.style.display = 'none';
      return;
    }

    if (panel) panel.style.display = '';
    if (badge) badge.textContent = markers.length;

    el.innerHTML = markers.map(m => {
      const priorityClass = m.priority === 'urgent' ? 'marker-priority-urgent' :
                           m.priority === 'high' ? 'marker-priority-high' : '';
      return `<div class="marker-item ${priorityClass}">
        <div><span class="marker-from">${esc(m.from_node)}</span> flagged a ${esc(m.target_type)}</div>
        <div class="marker-message">${esc(m.message)}</div>
        <div class="marker-target">${esc(m.target_path || m.target_id)}</div>
        <div class="marker-actions">
          <button onclick="window._markerDone('${esc(m.id)}')">Done</button>
          <button onclick="window._markerSeen('${esc(m.id)}')">Seen</button>
        </div>
      </div>`;
    }).join('');
  }

  window._markerDone = async function(id) {
    await fetch('/api/markers/' + id + '/done', {method: 'POST'});
    refreshAll();
  };
  window._markerSeen = async function(id) {
    await fetch('/api/markers/' + id + '/seen', {method: 'POST'});
    refreshAll();
  };

  // --- Conversations ---
  let _convSearchWired = false;
  function setupConvSearch() {
    if (_convSearchWired) return;
    _convSearchWired = true;
    const input = document.getElementById('conv-search-input');
    const btn = document.getElementById('conv-search-btn');
    if (btn) {
      btn.addEventListener('click', () => searchConversations(input.value));
      input.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') searchConversations(input.value);
      });
    }
  }

  async function loadConversations() {
    setupConvSearch();
    try {
      const peerGroups = await fetchJSON('/api/conversations/by-peer');
      const el = document.getElementById('conv-list');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No conversations saved yet</div>';
        return;
      }
      let html = '';
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];
        html += `<div class="tree-node peer-header clickable" data-conv-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-conv-peer-children="${pi}">`;
        for (let si = 0; si < pg.sessions.length; si++) {
          const sg = pg.sessions[si];
          html += `<div class="tree-node session-header clickable" data-conv-session="${pi}-${si}">
            <span class="tree-arrow" style="font-size:10px">&#x25BC;</span>
            <span class="status-dot green" style="width:6px;height:6px"></span>
            <span>${esc(sg.session)}</span>
            <span class="count">${sg.count}</span>
          </div>`;
          html += `<div class="session-children" data-conv-session-children="${pi}-${si}">`;
          for (const c of sg.conversations) {
            html += `<div class="conv-item" data-id="${esc(c.id)}">
              <div class="conv-item-title">${esc(c.title)}</div>
              <div class="conv-item-meta">
                <span>${c.turn_count} turns</span>
                <span>${formatTime(c.updated_at)}</span>
              </div>
            </div>`;
          }
          html += `</div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer toggle
      el.querySelectorAll('[data-conv-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.convPeer;
          const children = el.querySelector(`[data-conv-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Session toggle
      el.querySelectorAll('[data-conv-session]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.convSession;
          const children = el.querySelector(`[data-conv-session-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Conversation clicks
      el.querySelectorAll('.conv-item').forEach(item => {
        item.addEventListener('click', () => {
          el.querySelectorAll('.conv-item').forEach(i => i.classList.remove('active'));
          item.classList.add('active');
          window._loadConv(item.dataset.id);
        });
      });
    } catch (e) {
      console.error('Load conversations failed:', e);
    }
  }

  function renderConversationSearchResults(convos) {
    const el = document.getElementById('conv-list');
    if (!convos.length) {
      el.innerHTML = '<div class="empty-state">No conversations found</div>';
      return;
    }
    el.innerHTML = convos.map(c => `<div class="conv-item" data-id="${esc(c.id)}">
      <div class="conv-item-title">${esc(c.title)}</div>
      <div class="conv-item-meta">
        <span class="conv-item-agent">${esc(c.agent || 'unknown')}</span>
        <span>${c.turn_count} turns</span>
      </div>
    </div>`).join('');
    el.querySelectorAll('.conv-item').forEach(item => {
      item.addEventListener('click', () => {
        el.querySelectorAll('.conv-item').forEach(i => i.classList.remove('active'));
        item.classList.add('active');
        window._loadConv(item.dataset.id);
      });
    });
  }

  // Expose to onclick
  window._loadConv = async function(id) {
    // Highlight active
    document.querySelectorAll('.conv-item').forEach(i => i.classList.remove('active'));
    const active = document.querySelector(`.conv-item[data-id="${id}"]`);
    if (active) active.classList.add('active');

    try {
      const conv = await fetchJSON('/api/conversations/' + id);
      renderConversationDetail(conv);
    } catch (e) {
      console.error('Load conversation failed:', e);
    }
  };

  async function renderConversationDetail(conv) {
    const titleEl = document.getElementById('conv-detail-title');
    const bodyEl = document.getElementById('conv-detail');

    const convRef = conv.id ? conv.id.substring(0, 12) : '';
    titleEl.innerHTML = `${esc(conv.title || 'Conversation')} <button class="btn-copy" onclick="window._copy('recall conversation ${convRef}')" title="Copy reference">&#x2398;</button>`;

    // Fetch linked actions
    let actionsHtml = '';
    try {
      const actions = await fetchJSON('/api/conversations/' + conv.id + '/actions');
      if (actions && actions.length) {
        actionsHtml = `<div class="conv-turn" style="background:var(--bg-card);border-bottom:2px solid var(--border)">
          <div class="conv-turn-header">
            <span class="conv-turn-role" style="color:var(--yellow)">ACTIONS</span>
            <span class="conv-turn-index">${actions.length} tool calls</span>
          </div>
          <div style="display:flex;flex-wrap:wrap;gap:4px;margin-top:4px">
            ${actions.slice(0, 50).map(a => `<span class="badge type action-link" style="cursor:pointer;font-size:10px" data-aid="${esc(a.id)}">${esc(a.tool_name)}</span>`).join('')}
          </div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:6px">Click a badge to view the action detail</div>
        </div>`;
      }
    } catch (e) {}

    if (!conv.turns || !conv.turns.length) {
      bodyEl.innerHTML = actionsHtml + '<div class="empty-state">No turns in this conversation</div>';
      bindActionLinks(bodyEl);
      return;
    }

    bodyEl.innerHTML = actionsHtml + conv.turns.map((t, i) => {
      let label = 'You';
      if (t.role === 'assistant') {
        label = formatModel(t.model) || 'Assistant';
      }
      return `
      <div class="conv-turn ${esc(t.role)}">
        <div class="conv-turn-header">
          <span class="conv-turn-role">${esc(label)}</span>
          <span class="conv-turn-index">#${i + 1}</span>
          <button class="btn-copy" onclick="window._copy('recall conversation ${convRef} turn ${i + 1}: ${esc(t.content.substring(0, 80)).replace(/'/g, "")}')" title="Copy this turn">&#x2398;</button>
        </div>
        <div class="conv-turn-content">${esc(t.content)}</div>
      </div>`;
    }).join('');

    bindActionLinks(bodyEl);
  }

  function bindActionLinks(container) {
    container.querySelectorAll('.action-link').forEach(el => {
      el.addEventListener('click', () => {
        switchView('actions');
        setTimeout(() => loadActionDetail(el.dataset.aid), 300);
      });
    });
  }

  window._copy = function(text) {
    navigator.clipboard.writeText(text).then(() => {
      // Brief visual feedback
      const toast = document.createElement('div');
      toast.className = 'copy-toast';
      toast.textContent = 'Copied!';
      document.body.appendChild(toast);
      setTimeout(() => toast.remove(), 1500);
    });
  };

  async function searchConversations(query) {
    if (!query.trim()) {
      loadConversations();
      return;
    }
    try {
      const resp = await fetch('/api/conversations/search', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({query, limit: 20})
      });
      const results = await resp.json();
      const convos = (results || []).map(r => ({
        id: r.conversation.id,
        title: r.conversation.title + ` (${r.score.toFixed(2)})`,
        agent: r.conversation.agent,
        turn_count: r.conversation.turn_count,
      }));
      renderConversationSearchResults(convos);
    } catch (e) {
      console.error('Search conversations failed:', e);
    }
  }

  function formatDate(dateStr) {
    const d = new Date(dateStr);
    const today = new Date();
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);

    if (dateStr === today.toISOString().substring(0, 10)) return 'Today';
    if (dateStr === yesterday.toISOString().substring(0, 10)) return 'Yesterday';
    return d.toLocaleDateString('en-US', {weekday: 'long', month: 'short', day: 'numeric'});
  }

  function formatTime(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleTimeString('en-US', {hour: '2-digit', minute: '2-digit'});
  }

  // --- Actions ---
  async function loadActions() {
    const el = document.getElementById('actions-tree');
    try {
      const peerGroups = await fetchJSON('/api/actions/by-peer');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No actions recorded yet. Actions appear as sessions use tools (Bash, Read, Edit, etc.)</div>';
        return;
      }
      let html = '';
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];
        html += `<div class="tree-node peer-header clickable" data-action-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-action-peer-children="${pi}">`;
        for (let si = 0; si < pg.sessions.length; si++) {
          const sg = pg.sessions[si];
          const sid = sg.session.length > 12 ? sg.session.substring(0, 12) : sg.session;
          html += `<div class="tree-node session-header clickable" data-action-session="${pi}-${si}">
            <span class="tree-arrow" style="font-size:10px">&#x25BC;</span>
            <span class="status-dot green" style="width:6px;height:6px"></span>
            <span>${esc(sid)}</span>
            <span class="count">${sg.count}</span>
          </div>`;
          html += `<div class="session-children" data-action-session-children="${pi}-${si}" style="display:none">`;
          for (const a of sg.actions) {
            html += `<div class="tree-node peer-leaf clickable" data-action-id="${esc(a.id)}">
              <span class="badge type" style="font-size:10px">${esc(a.tool_name)}</span>
              <span style="font-size:11px;color:var(--text-primary);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(a.tool_input.length > 40 ? a.tool_input.substring(0,40) + '...' : a.tool_input)}</span>
              <span style="color:var(--text-muted);font-size:10px">${formatTime(a.created_at)}</span>
            </div>`;
          }
          html += `</div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer toggle
      el.querySelectorAll('[data-action-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.actionPeer;
          const children = el.querySelector(`[data-action-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Session toggle
      el.querySelectorAll('[data-action-session]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.actionSession;
          const children = el.querySelector(`[data-action-session-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Action leaf click — fetch full detail
      el.querySelectorAll('.tree-node.peer-leaf').forEach(node => {
        node.addEventListener('click', (e) => {
          if (e.target.closest('.tree-arrow')) return;
          el.querySelectorAll('.tree-node').forEach(n => n.classList.remove('active'));
          node.classList.add('active');
          loadActionDetail(node.dataset.actionId);
        });
      });
    } catch (e) {
      console.error('Load actions failed:', e);
    }
  }

  async function loadActionDetail(id) {
    try {
      const a = await fetchJSON('/api/actions/' + id);
      if (!a) return;
      const titleEl = document.getElementById('action-detail-title');
      const bodyEl = document.getElementById('action-detail');
      const sid = a.session_id ? (a.session_id.length > 12 ? a.session_id.substring(0, 12) : a.session_id) : '';
      const convId = a.session_id ? 'hook-' + a.session_id : '';

      titleEl.textContent = a.tool_name;
      bodyEl.innerHTML = `
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px">
          <span class="badge type">${esc(a.tool_name)}</span>
          <span class="session-uuid">${esc(sid)}</span>
          ${a.source_node ? `<span class="badge" style="color:var(--accent)">${esc(a.source_node)}</span>` : ''}
          <span style="color:var(--text-muted);font-size:11px;margin-left:auto">${a.created_at ? new Date(a.created_at).toLocaleString() : ''}</span>
        </div>
        ${convId ? `<div style="margin-bottom:8px">
          <span style="font-size:12px;color:var(--text-muted)">Conversation:</span>
          <span class="badge scope conv-nav" style="cursor:pointer" data-conv-id="${esc(convId)}">session ${esc(sid)}</span>
        </div>` : ''}
        ${a.task_id ? `<div style="margin-bottom:12px">
          <span style="font-size:12px;color:var(--text-muted)">Task:</span>
          <span class="badge tag task-nav" style="cursor:pointer" data-tid="${esc(a.task_id)}">${esc(a.task_id.substring(0,12))}</span>
        </div>` : ''}
        <div style="font-size:12px;color:var(--text-muted);margin-bottom:6px;font-weight:500">Input</div>
        <div class="action-input" style="max-height:400px;overflow-y:auto">${esc(a.tool_input)}</div>
        ${a.tool_response ? `
          <div style="font-size:12px;color:var(--text-muted);margin:12px 0 6px;font-weight:500">Response</div>
          <div class="action-response" style="max-height:400px;overflow-y:auto">${esc(a.tool_response)}</div>
        ` : ''}
        ${a.cwd ? `<div style="margin-top:12px;font-family:var(--font-mono);font-size:11px;color:var(--text-muted)">CWD: ${esc(a.cwd)}</div>` : ''}
        <div style="margin-top:8px;font-family:var(--font-mono);font-size:10px;color:var(--text-muted)">Session: ${esc(a.session_id)}</div>
      `;

      // Bind conversation link
      bodyEl.querySelectorAll('.conv-nav').forEach(el => {
        el.addEventListener('click', () => {
          switchView('conversations');
          setTimeout(() => window._loadConv(el.dataset.convId), 300);
        });
      });

      // Bind task link
      bodyEl.querySelectorAll('.task-nav').forEach(el => {
        el.addEventListener('click', () => {
          switchView('tasks');
          setTimeout(() => loadTaskDetail(el.dataset.tid), 300);
        });
      });
    } catch (e) {
      console.error('Load action detail failed:', e);
    }
  }

  // --- Products ---
  async function loadProducts() {
    const el = document.getElementById('products-list');
    try {
      const peerGroups = await fetchJSON('/api/products/by-peer');
      let html = `<div style="padding:8px 0;margin-bottom:8px"><button class="btn-search" id="btn-new-product" style="font-size:12px;padding:6px 16px">+ New Product</button></div>`;
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = html + '<div class="empty-state" style="padding:16px">No products yet. Create one to group your manifests.</div>';
        el.querySelector('#btn-new-product').addEventListener('click', () => window._createProduct());
        return;
      }

      // Peer → Product → Manifests (mirrors amnesia: Peer → Session → Violations)
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];

        // Peer header (L1)
        html += `<div class="tree-node peer-header clickable" data-prod-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-prod-peer-children="${pi}">`;

        // Product headers (L2) — same as amnesia sessions
        for (let pri = 0; pri < pg.products.length; pri++) {
          const p = pg.products[pri];
          const dotColor = p.status === 'open' ? 'green' : p.status === 'closed' ? 'red' : p.status === 'archive' ? 'red' : 'yellow';
          const metaParts = [];
          if (p.total_tasks > 0) metaParts.push(`${p.total_tasks} tasks`);
          if (p.total_turns > 0) metaParts.push(`${p.total_turns} turns`);
          if (p.total_cost > 0) metaParts.push(`$${p.total_cost.toFixed(2)}`);

          html += `<div class="tree-node clickable" style="padding-left:24px" data-prod-item="${pi}-${pri}" data-product-id="${esc(p.id)}">
            <span class="tree-arrow">&#x25B6;</span>
            <span class="status-dot ${dotColor}"></span>
            <span>${esc(p.title)}</span>
            <span class="session-uuid">${esc(p.marker)}</span>
            ${p.total_manifests > 0 ? `<span class="count">${p.total_manifests}</span>` : ''}
          </div>`;
          html += `<div style="display:none" data-prod-item-children="${pi}-${pri}">`;

          // Manifest placeholder — loaded on expand
          html += `<div class="prod-manifests-placeholder" data-prod-manifests-for="${esc(p.id)}" style="margin-left:24px">
            <div style="padding:8px 12px;color:var(--text-muted);font-size:12px">
              ${metaParts.length ? metaParts.join(' &middot; ') : ''}
              ${p.total_manifests > 0 ? '<div style="margin-top:4px;font-style:italic">Loading manifests...</div>' : '<div style="margin-top:4px;font-style:italic">No manifests linked</div>'}
            </div>
          </div>`;
          html += `</div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer toggle (L1)
      el.querySelectorAll('[data-prod-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.prodPeer;
          const children = el.querySelector(`[data-prod-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Product toggle (L2) — expand loads manifests, click also opens detail
      el.querySelectorAll('[data-prod-item]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.prodItem;
          const productId = node.dataset.productId;
          const children = el.querySelector(`[data-prod-item-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');

          // Load detail panel
          loadProductDetail(productId);

          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
            // Lazy-load manifests on first expand
            const placeholder = children.querySelector('.prod-manifests-placeholder');
            if (placeholder) {
              loadProductManifests(productId, placeholder);
            }
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // New product button
      const newBtn = el.querySelector('#btn-new-product');
      if (newBtn) newBtn.addEventListener('click', () => window._createProduct());
    } catch (e) {
      console.error('Load products failed:', e);
    }
  }

  async function loadProductManifests(productId, container) {
    try {
      const manifests = await fetchJSON('/api/products/' + productId + '/manifests');
      if (!manifests || !manifests.length) {
        container.innerHTML = '<div style="padding:8px 12px;color:var(--text-muted);font-size:12px;font-style:italic">No manifests linked</div>';
        return;
      }
      let html = '';
      for (const m of manifests) {
        const statusClass = m.status === 'open' ? 'confirmed' : m.status === 'closed' ? 'flagged' : 'dismissed';
        html += `<div class="amnesia-item ${statusClass}" style="cursor:pointer" onclick="switchView('manifests');setTimeout(()=>window._loadManifest&&window._loadManifest('${esc(m.id)}'),300)">
          <div class="amnesia-header">
            <span class="amnesia-status-label">${esc(m.status)}</span>
            <span class="session-uuid">${esc(m.marker)}</span>
            <span style="color:var(--text-muted);font-size:11px;margin-left:auto">${formatTime(m.updated_at)}</span>
          </div>
          <div class="amnesia-rule">${esc(m.title)}</div>
        </div>`;
      }
      container.innerHTML = html;
    } catch (e) {
      container.innerHTML = '<div style="padding:8px 12px;color:var(--red);font-size:12px">Failed to load manifests</div>';
    }
  }

  // Create product dialog
  window._createProduct = function() {
    const titleEl = document.getElementById('product-detail-title');
    const bodyEl = document.getElementById('product-detail');
    titleEl.textContent = 'New Product';
    bodyEl.innerHTML = `
      <div style="max-width:500px">
        <div style="margin-bottom:12px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Title *</label>
          <input id="new-product-title" class="conv-search" style="width:100%;padding:8px 12px;font-size:14px" placeholder="Product name" autofocus>
        </div>
        <div style="margin-bottom:12px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Description</label>
          <textarea id="new-product-desc" class="conv-search" style="width:100%;min-height:80px;padding:8px 12px;font-size:13px;resize:vertical" placeholder="What is this product/project about?"></textarea>
        </div>
        <div style="margin-bottom:16px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Tags (comma-separated)</label>
          <input id="new-product-tags" class="conv-search" style="width:100%;padding:8px 12px;font-size:13px" placeholder="e.g. openloom, backend">
        </div>
        <div style="display:flex;gap:8px">
          <button class="btn-search" id="btn-save-product" style="padding:6px 20px;font-size:13px">Create Product</button>
          <button class="btn-dismiss" onclick="loadProducts()" style="padding:6px 16px;font-size:13px">Cancel</button>
        </div>
      </div>`;
    document.getElementById('btn-save-product').addEventListener('click', async () => {
      const title = document.getElementById('new-product-title').value.trim();
      if (!title) { alert('Title is required'); return; }
      const desc = document.getElementById('new-product-desc').value.trim();
      const tags = document.getElementById('new-product-tags').value.split(',').map(t => t.trim()).filter(Boolean);
      try {
        const p = await fetchJSON('/api/products', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({title, description: desc, tags}) });
        loadProducts();
        if (p && p.id) setTimeout(() => loadProductDetail(p.id), 300);
      } catch(e) { alert('Error: ' + e.message); }
    });
    document.getElementById('new-product-title').focus();
  };

  window._loadProduct = loadProductDetail;
  async function loadProductDetail(id) {
    document.querySelectorAll('[data-product-id]').forEach(i => i.classList.remove('active'));
    const active = document.querySelector(`[data-product-id="${id}"]`);
    if (active) active.classList.add('active');

    try {
      const p = await fetchJSON('/api/products/' + id);
      if (!p) return;

      const titleEl = document.getElementById('product-detail-title');
      const bodyEl = document.getElementById('product-detail');

      titleEl.textContent = p.title;

      const tags = (p.tags || []).map(t => `<span class="badge tag">${esc(t)}</span>`).join(' ');
      const statusClass = p.status === 'open' ? 'scope' : p.status === 'closed' ? 'type' : p.status === 'draft' ? '' : p.status === 'archive' ? 'type' : '';

      // Fetch linked manifests
      let manifestsHtml = '';
      try {
        const manifests = await fetchJSON('/api/products/' + p.id + '/manifests');
        if (manifests && manifests.length) {
          const manifestRows = manifests.map(m => {
            const mStatusClass = m.status === 'open' ? 'scope' : m.status === 'closed' ? 'type' : m.status === 'archive' ? 'type' : '';
            const mCostParts = [];
            if (m.total_tasks > 0) mCostParts.push(`${m.total_tasks} tasks`);
            if (m.total_turns > 0) mCostParts.push(`${m.total_turns} turns`);
            if (m.total_cost > 0) mCostParts.push(`<span style="color:var(--green)">$${m.total_cost.toFixed(2)}</span>`);
            return `<div class="manifest-item" style="border-bottom:1px solid var(--border);padding:10px 12px;display:flex;align-items:center">
              <div style="flex:1;cursor:pointer" onclick="switchView('manifests');setTimeout(()=>window._loadManifest('${esc(m.id)}'),300)">
                <div style="display:flex;align-items:center;gap:8px">
                  <span class="session-uuid" style="font-size:11px">${esc(m.marker)}</span>
                  <span class="badge ${mStatusClass}" style="font-size:10px">${esc(m.status)}</span>
                  ${mCostParts.length ? mCostParts.join('<span style="opacity:0.3;font-size:10px"> | </span>') : ''}
                </div>
                <div style="font-size:13px;color:var(--text-primary);margin-top:4px">${esc(m.title)}</div>
              </div>
              <button class="btn-dismiss" style="font-size:10px;padding:2px 8px;flex-shrink:0" onclick="event.stopPropagation();window._unlinkManifestFromProduct('${esc(m.id)}','${esc(p.id)}')" title="Remove from product">✕</button>
            </div>`;
          }).join('');
          manifestsHtml = `<div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="font-size:13px;color:var(--text-primary);margin-bottom:8px;font-weight:600">Linked Manifests <span style="font-weight:400;font-size:12px;color:var(--text-muted)">(${manifests.length})</span></div>
            <div style="border:1px solid var(--border);border-radius:4px;overflow:hidden">${manifestRows}</div>
          </div>`;
        } else {
          manifestsHtml = `<div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="font-size:13px;color:var(--text-primary);margin-bottom:8px;font-weight:600">Linked Manifests</div>
            <div style="font-size:12px;color:var(--text-muted);padding:12px;border:1px dashed var(--border);border-radius:4px;text-align:center">No manifests linked yet</div>
          </div>`;
        }
      } catch(e) {}

      // Fetch linked ideas
      let ideasHtml = '';
      try {
        const ideas = await fetchJSON('/api/products/' + p.id + '/ideas');
        if (ideas && ideas.length) {
          const ideaRows = ideas.map(i => {
            const prioColor = i.priority === 'critical' ? 'var(--red)' : i.priority === 'high' ? 'var(--yellow)' : 'var(--text-muted)';
            return `<div class="manifest-item" style="border-bottom:1px solid var(--border);padding:8px 12px;display:flex;align-items:center">
              <div style="flex:1">
                <div style="display:flex;align-items:center;gap:8px">
                  <span class="session-uuid" style="font-size:11px">${esc(i.marker)}</span>
                  <span class="badge" style="font-size:10px">${esc(i.status)}</span>
                  <span style="font-size:10px;color:${prioColor}">${esc(i.priority)}</span>
                </div>
                <div style="font-size:13px;color:var(--text-primary);margin-top:2px">${esc(i.title)}</div>
              </div>
              <button class="btn-dismiss" style="font-size:10px;padding:2px 8px;flex-shrink:0" onclick="event.stopPropagation();window._unlinkIdeaFromProduct('${esc(i.id)}','${esc(p.id)}')" title="Remove from product">✕</button>
            </div>`;
          }).join('');
          ideasHtml = `<div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span style="font-size:13px;color:var(--text-primary);font-weight:600">Linked Ideas <span style="font-weight:400;font-size:12px;color:var(--text-muted)">(${ideas.length})</span></span>
              <button class="btn-search" style="font-size:11px;padding:2px 10px" onclick="window._linkIdeaToProduct('${esc(p.id)}')">+ Link Idea</button>
            </div>
            <div style="border:1px solid var(--border);border-radius:4px;overflow:hidden">${ideaRows}</div>
          </div>`;
        } else {
          ideasHtml = `<div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span style="font-size:13px;color:var(--text-primary);font-weight:600">Linked Ideas</span>
              <button class="btn-search" style="font-size:11px;padding:2px 10px" onclick="window._linkIdeaToProduct('${esc(p.id)}')">+ Link Idea</button>
            </div>
            <div style="font-size:12px;color:var(--text-muted);padding:12px;border:1px dashed var(--border);border-radius:4px;text-align:center">No ideas linked yet</div>
          </div>`;
        }
      } catch(e) {}

      bodyEl.innerHTML = `
        <div>
          <!-- BREADCRUMB -->
          <div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;font-family:var(--font-mono)">
            <span style="cursor:pointer;color:var(--accent)" onclick="switchView('products')">${esc(p.source_node ? p.source_node.substring(0,12) : 'node')}</span>
            <span style="opacity:0.4"> &rarr; </span>
            <span style="color:var(--text-primary)">${esc(p.marker)} ${esc(p.title)}</span>
          </div>
          <!-- METADATA BAR -->
          <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:16px">
            <span class="session-uuid" style="font-size:14px">${esc(p.marker)}</span>
            <span class="badge ${statusClass}">${esc(p.status)}</span>
            ${tags}
            <button class="btn-copy" onclick="window._copy('get product ${esc(p.marker)}')" title="Copy ref">&#x2398;</button>
          </div>
          <!-- METRICS BAR -->
          <div style="display:flex;gap:12px;font-size:12px;color:var(--text-muted);margin-bottom:12px;align-items:center;flex-wrap:wrap;padding:8px 12px;background:var(--bg-secondary);border:1px solid var(--border);border-radius:6px;font-family:var(--font-mono)">
            <span>Manifests: <strong style="color:var(--text-primary)">${p.total_manifests || 0}</strong></span>
            <span>Tasks: <strong style="color:var(--text-primary)">${p.total_tasks || 0}</strong></span>
            <span>Turns: <strong style="color:var(--text-primary)">${p.total_turns || 0}</strong></span>
            <span>Cost: <strong style="color:var(--green)">$${(p.total_cost || 0).toFixed(2)}</strong></span>
            <span style="opacity:0.3">|</span>
            <span>Created: ${new Date(p.created_at).toLocaleString()}</span>
            <span>Updated: ${new Date(p.updated_at).toLocaleString()}</span>
          </div>
          <!-- CONTROLS -->
          <div style="display:flex;gap:8px;margin-bottom:12px;flex-wrap:wrap">
            <button class="btn-search" style="font-size:11px;padding:4px 12px" onclick="window._editProduct('${esc(p.id)}')">&#9998; Edit</button>
            <button class="btn-search" style="font-size:11px;padding:4px 12px;background:var(--bg-input)" onclick="window._linkManifestToProduct('${esc(p.id)}')">+ Link Manifest</button>
            ${['draft','open','closed','archive'].map(s => {
              const active = p.status === s;
              const color = s === 'open' ? 'var(--green)' : s === 'closed' ? 'var(--text-muted)' : s === 'archive' ? 'var(--red)' : 'var(--yellow)';
              return '<button class="product-status-btn" data-status="' + s + '" style="padding:4px 12px;font-size:11px;font-weight:600;text-transform:uppercase;border-radius:4px;cursor:pointer;border:1px solid ' + color + ';background:' + (active ? color : 'transparent') + ';color:' + (active ? 'var(--bg-primary)' : color) + ';opacity:' + (active ? '1' : '0.7') + '" onclick="window._updateProductStatus(\'' + esc(p.id) + '\',\'' + s + '\')">' + s + '</button>';
            }).join('')}
          </div>
          ${p.description ? `<div style="font-size:13px;color:var(--text-secondary);line-height:1.6;margin-bottom:12px;white-space:pre-wrap">${esc(p.description)}</div>` : ''}
          <!-- DIAGRAM BUTTON -->
          <div style="margin-bottom:12px">
            <button class="btn-search" style="font-size:12px;padding:6px 16px" onclick="window._showProductDiagram('${esc(p.id)}','${esc(p.title)}')">&#x25C8; Product DAG</button>
          </div>
          ${manifestsHtml}
          ${ideasHtml}
        </div>`;

    } catch (e) {
      console.error('Load product detail failed:', e);
    }
  }

  // --- Cytoscape Directed Acyclic Graph Diagram (Full Page) ---
  window._showProductDiagram = function(productId, productTitle) {
    // Create full-page overlay
    let overlay = document.getElementById('product-diagram-overlay');
    if (overlay) overlay.remove();
    overlay = document.createElement('div');
    overlay.id = 'product-diagram-overlay';
    overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;z-index:1000;background:var(--bg-primary);display:flex;flex-direction:column';
    overlay.innerHTML = `
      <div style="display:flex;align-items:center;gap:12px;padding:12px 20px;border-bottom:1px solid var(--border);background:var(--bg-secondary);flex-shrink:0">
        <button id="diagram-back-btn" style="padding:6px 14px;font-size:12px;font-weight:600;border:1px solid var(--border);border-radius:4px;cursor:pointer;background:var(--bg-input);color:var(--text-primary)">&#x2190; Back</button>
        <span style="font-size:15px;font-weight:600;color:var(--text-primary)">${esc(productTitle)}</span>
        <span style="font-size:12px;color:var(--text-muted)">Directed Acyclic Graph</span>
        <span style="margin-left:auto;display:flex;align-items:center;gap:16px;font-size:11px;color:var(--text-muted)">
          <span style="display:flex;align-items:center;gap:4px"><span style="display:inline-block;width:20px;height:0;border-top:2px solid rgba(255,255,255,0.15)"></span>hierarchy</span>
          <span style="display:flex;align-items:center;gap:4px"><span style="display:inline-block;width:20px;height:0;border-top:2px dashed #f59e0b"></span>depends on</span>
          <span>Scroll to zoom &middot; Drag to pan &middot; Click node to drill down</span>
        </span>
      </div>
      <div id="product-cytoscape" style="flex:1;width:100%"></div>
    `;
    document.body.appendChild(overlay);
    document.getElementById('diagram-back-btn').addEventListener('click', () => overlay.remove());
    // ESC to close
    const escHandler = (e) => { if (e.key === 'Escape') { overlay.remove(); document.removeEventListener('keydown', escHandler); } };
    document.addEventListener('keydown', escHandler);
    renderProductDiagram(productId);
  };

  async function renderProductDiagram(productId) {
    const container = document.getElementById('product-cytoscape');
    if (!container || typeof cytoscape === 'undefined') return;
    container.innerHTML = '<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--text-muted)">Loading...</div>';

    try {
      const data = await fetchJSON('/api/products/' + productId + '/hierarchy');
      if (!data) return;

      const elements = [];
      const statusColor = (type, status) => {
        if (type === 'task') {
          if (status === 'completed') return '#00d97e';
          if (status === 'running') return '#f5c542';
          if (status === 'failed') return '#e63757';
          if (status === 'scheduled' || status === 'waiting') return '#f5c542';
          return '#71717a';
        }
        // product + manifest
        if (status === 'closed' || status === 'archive') return '#00d97e';
        if (status === 'open') return '#3b82f6';
        if (status === 'draft') return '#f5c542';
        return '#71717a';
      };

      const nodeShape = (type) => type === 'product' ? 'round-rectangle' : type === 'manifest' ? 'round-rectangle' : 'ellipse';
      const nodeSize = (type) => type === 'product' ? 60 : type === 'manifest' ? 45 : 30;

      const nodeLabel = (type) => {
        return type === 'product' ? 'Product' : type === 'manifest' ? 'Manifest' : 'Task';
      };

      // Add product node
      elements.push({ data: {
        id: data.id, label: nodeLabel(data.type), title: data.title, type: data.type, status: data.status,
        marker: data.marker, color: statusColor(data.type, data.status),
        nodeSize: nodeSize(data.type), meta: JSON.stringify(data.meta || {})
      }});

      // Add manifest + task nodes
      for (const m of (data.children || [])) {
        elements.push({ data: {
          id: m.id, label: nodeLabel(m.type), title: m.title, type: m.type, status: m.status,
          marker: m.marker, color: statusColor(m.type, m.status),
          nodeSize: nodeSize(m.type), meta: JSON.stringify(m.meta || {})
        }});
        elements.push({ data: { source: data.id, target: m.id } });

        for (const t of (m.children || [])) {
          elements.push({ data: {
            id: t.id, label: nodeLabel(t.type), title: t.title, type: t.type, status: t.status,
            marker: t.marker, color: statusColor(t.type, t.status),
            nodeSize: nodeSize(t.type), meta: JSON.stringify(t.meta || {}),
            depends_on: t.depends_on || ''
          }});
          // Only connect task to manifest if it has no dependency — chained tasks connect to their predecessor instead
          if (!t.depends_on) {
            elements.push({ data: { source: m.id, target: t.id } });
          }
        }
      }

      // Add dependency edges between tasks (dashed, orange)
      for (const el of [...elements]) {
        const d = el.data;
        if (d && d.type === 'task' && d.depends_on) {
          // Edge from dependency TO dependent (arrow shows execution order)
          const depExists = elements.some(e => e.data && e.data.id === d.depends_on);
          if (depExists) {
            elements.push({ data: { source: d.depends_on, target: d.id, edgeType: 'dependency' } });
          }
        }
      }

      container.innerHTML = '';
      const cy = cytoscape({
        container: container,
        elements: elements,
        layout: {
          name: 'dagre',
          rankDir: 'TB',
          spacingFactor: 1.4,
          nodeSep: 30,
          rankSep: 60,
        },
        style: [
          {
            selector: 'node',
            style: {
              'label': 'data(label)',
              'text-wrap': 'wrap',
              'text-max-width': '120px',
              'font-size': '10px',
              'text-valign': 'center',
              'text-halign': 'center',
              'color': '#e4e4e7',
              'background-color': 'data(color)',
              'border-width': 2,
              'border-color': 'data(color)',
              'width': 'data(nodeSize)',
              'height': 'data(nodeSize)',
              'shape': 'round-rectangle',
              'text-outline-color': '#0a0a0f',
              'text-outline-width': 2,
            }
          },
          {
            selector: 'node[type="task"]',
            style: { 'shape': 'ellipse', 'font-size': '8px', 'text-max-width': '90px' }
          },
          {
            selector: 'node[type="product"]',
            style: { 'font-size': '12px', 'font-weight': 'bold', 'text-max-width': '160px' }
          },
          {
            selector: 'edge',
            style: {
              'width': 1.5,
              'line-color': 'rgba(255,255,255,0.15)',
              'target-arrow-color': 'rgba(255,255,255,0.15)',
              'target-arrow-shape': 'triangle',
              'curve-style': 'bezier',
              'arrow-scale': 0.8,
            }
          },
          {
            selector: 'edge[edgeType="dependency"]',
            style: {
              'width': 2,
              'line-color': '#f59e0b',
              'line-style': 'dashed',
              'line-dash-pattern': [6, 3],
              'target-arrow-color': '#f59e0b',
              'target-arrow-shape': 'triangle',
              'curve-style': 'bezier',
              'arrow-scale': 0.9,
            }
          },
          {
            selector: 'node:active, node:selected',
            style: { 'border-width': 3, 'border-color': '#3b82f6', 'overlay-opacity': 0 }
          },
        ],
        minZoom: 0.3,
        maxZoom: 3,
        wheelSensitivity: 0.3,
      });

      // Tooltip
      const tooltip = document.createElement('div');
      tooltip.style.cssText = 'position:absolute;display:none;background:rgba(10,10,15,0.95);border:1px solid rgba(255,255,255,0.15);border-radius:6px;padding:8px 12px;font-size:11px;color:#e4e4e7;pointer-events:none;z-index:100;max-width:250px;font-family:var(--font-mono)';
      container.style.position = 'relative';
      container.appendChild(tooltip);

      cy.on('mouseover', 'node', (e) => {
        const node = e.target;
        const d = node.data();
        const meta = JSON.parse(d.meta || '{}');
        let html = `<div style="font-weight:600;margin-bottom:4px">${esc(d.title)}</div>`;
        html += `<div><span style="color:var(--text-muted)">Status:</span> <span style="color:${d.color}">${d.status}</span></div>`;
        html += `<div style="color:var(--text-muted);font-size:10px;margin-bottom:4px">${d.marker}</div>`;
        if (d.type === 'product') {
          html += `<div>${meta.total_manifests || 0} manifests</div>`;
          html += `<div>${meta.total_tasks || 0} tasks</div>`;
        } else if (d.type === 'manifest') {
          html += `<div>${meta.total_tasks || 0} tasks</div>`;
        } else {
          html += `<div>${meta.turns || 0} turns</div>`;
          if (meta.run_count > 0) html += `<div>${meta.run_count} runs</div>`;
          if (d.depends_on) {
            const depNode = cy.getElementById(d.depends_on);
            const depTitle = depNode.length ? depNode.data('title') : d.depends_on.slice(0, 8);
            html += `<div style="color:#f59e0b">depends on: ${esc(depTitle)}</div>`;
          }
        }
        if (meta.total_cost > 0 || meta.cost_usd > 0) html += `<div><span style="color:var(--green)">$${(meta.total_cost || meta.cost_usd || 0).toFixed(2)}</span></div>`;
        tooltip.innerHTML = html;
        tooltip.style.display = '';
        const pos = node.renderedPosition();
        tooltip.style.left = (pos.x + 20) + 'px';
        tooltip.style.top = (pos.y - 20) + 'px';
      });
      cy.on('mouseout', 'node', () => { tooltip.style.display = 'none'; });

      // Click to drill down
      cy.on('tap', 'node', (e) => {
        const d = e.target.data();
        const overlay = document.getElementById('product-diagram-overlay');
        if (overlay) overlay.remove();
        if (d.type === 'product') {
          switchView('products');
          setTimeout(() => loadProductDetail(d.id), 300);
        } else if (d.type === 'manifest') {
          switchView('manifests');
          setTimeout(() => window._loadManifest(d.id), 300);
        } else if (d.type === 'task') {
          switchView('tasks');
          setTimeout(() => loadTaskDetail(d.id), 300);
        }
      });

    } catch (e) {
      container.innerHTML = '<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--red)">Failed to load diagram</div>';
      console.error('Diagram error:', e);
    }
  }

  // Product CRUD actions
  window._editProduct = function(id) {
    const titleEl = document.getElementById('product-detail-title');
    const bodyEl = document.getElementById('product-detail');
    fetchJSON('/api/products/' + id).then(p => {
      if (!p) return;
      titleEl.textContent = 'Edit Product';
      bodyEl.innerHTML = `
        <div style="max-width:500px">
          <div style="margin-bottom:12px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Title</label>
            <input id="edit-product-title" class="conv-search" style="width:100%;padding:8px 12px;font-size:14px" value="${esc(p.title)}">
          </div>
          <div style="margin-bottom:12px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Description</label>
            <textarea id="edit-product-desc" class="conv-search" style="width:100%;min-height:80px;padding:8px 12px;font-size:13px;resize:vertical">${esc(p.description)}</textarea>
          </div>
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Tags (comma-separated)</label>
            <input id="edit-product-tags" class="conv-search" style="width:100%;padding:8px 12px;font-size:13px" value="${esc((p.tags||[]).join(', '))}">
          </div>
          <div style="display:flex;gap:8px">
            <button class="btn-search" id="btn-update-product" style="padding:6px 20px;font-size:13px">Save</button>
            <button class="btn-dismiss" onclick="loadProductDetail('${esc(p.id)}')" style="padding:6px 16px;font-size:13px">Cancel</button>
          </div>
        </div>`;
      document.getElementById('btn-update-product').addEventListener('click', async () => {
        const title = document.getElementById('edit-product-title').value.trim();
        if (!title) { alert('Title is required'); return; }
        const description = document.getElementById('edit-product-desc').value.trim();
        const tags = document.getElementById('edit-product-tags').value.split(',').map(t => t.trim()).filter(Boolean);
        await fetchJSON('/api/products/' + p.id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({title, description, tags}) });
        loadProducts();
        loadProductDetail(p.id);
      });
    });
  };

  window._updateProductStatus = async function(id, status) {
    await fetchJSON('/api/products/' + id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({status}) });
    loadProducts();
    loadProductDetail(id);
  };

  // _deleteProduct removed — use status toggle to archive instead

  window._linkManifestToProduct = async function(productId) {
    // Fetch all manifests not yet linked to this product
    const allManifests = await fetchJSON('/api/manifests');
    const unlinked = (allManifests || []).filter(m => m.project_id !== productId);
    if (!unlinked.length) { alert('No manifests available to link'); return; }

    const bodyEl = document.getElementById('product-detail');
    const origHtml = bodyEl.innerHTML;
    let listHtml = unlinked.map(m =>
      `<div class="manifest-item clickable" data-link-mid="${esc(m.id)}" style="padding:8px 12px;border-bottom:1px solid var(--border)">
        <span class="session-uuid" style="font-size:11px">${esc(m.marker)}</span>
        <span class="badge" style="font-size:10px">${esc(m.status)}</span>
        <span style="font-size:12px">${esc(m.title)}</span>
      </div>`
    ).join('');

    bodyEl.innerHTML = `
      <div>
        <div style="font-size:14px;font-weight:600;margin-bottom:12px">Link Manifest to Product</div>
        <div style="margin-bottom:12px;font-size:12px;color:var(--text-muted)">Click a manifest to link it:</div>
        <div style="border:1px solid var(--border);border-radius:4px;max-height:400px;overflow-y:auto">${listHtml}</div>
        <button class="btn-dismiss" style="margin-top:12px;padding:6px 16px;font-size:13px" onclick="loadProductDetail('${esc(productId)}')">Cancel</button>
      </div>`;

    bodyEl.querySelectorAll('[data-link-mid]').forEach(item => {
      item.addEventListener('click', async () => {
        const mid = item.dataset.linkMid;
        await fetchJSON('/api/manifests/' + mid, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: productId}) });
        loadProducts();
        loadProductDetail(productId);
      });
    });
  };

  window._unlinkManifestFromProduct = async function(manifestId, productId) {
    await fetchJSON('/api/manifests/' + manifestId, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: ''}) });
    loadProducts();
    loadProductDetail(productId);
  };

  window._linkIdeaToProduct = async function(productId) {
    const allIdeas = await fetchJSON('/api/ideas');
    const unlinked = (allIdeas || []).filter(i => i.project_id !== productId);
    if (!unlinked.length) { alert('No ideas available to link'); return; }

    const bodyEl = document.getElementById('product-detail');
    let listHtml = unlinked.map(i =>
      `<div class="manifest-item clickable" data-link-iid="${esc(i.id)}" style="padding:8px 12px;border-bottom:1px solid var(--border)">
        <span class="session-uuid" style="font-size:11px">${esc(i.marker)}</span>
        <span class="badge" style="font-size:10px">${esc(i.status)}</span>
        <span style="font-size:10px;color:var(--text-muted)">${esc(i.priority)}</span>
        <span style="font-size:12px">${esc(i.title)}</span>
      </div>`
    ).join('');

    bodyEl.innerHTML = `
      <div>
        <div style="font-size:14px;font-weight:600;margin-bottom:12px">Link Idea to Product</div>
        <div style="margin-bottom:12px;font-size:12px;color:var(--text-muted)">Click an idea to link it:</div>
        <div style="border:1px solid var(--border);border-radius:4px;max-height:400px;overflow-y:auto">${listHtml}</div>
        <button class="btn-dismiss" style="margin-top:12px;padding:6px 16px;font-size:13px" onclick="loadProductDetail('${esc(productId)}')">Cancel</button>
      </div>`;

    bodyEl.querySelectorAll('[data-link-iid]').forEach(item => {
      item.addEventListener('click', async () => {
        const iid = item.dataset.linkIid;
        const idea = (allIdeas || []).find(i => i.id === iid);
        if (idea) {
          await fetchJSON('/api/ideas/' + iid, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: productId, title: idea.title, description: idea.description, status: idea.status, priority: idea.priority}) });
        }
        loadProducts();
        loadProductDetail(productId);
      });
    });
  };

  window._unlinkIdeaFromProduct = async function(ideaId, productId) {
    const idea = await fetchJSON('/api/ideas/' + ideaId);
    if (idea) {
      await fetchJSON('/api/ideas/' + ideaId, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: '', title: idea.title, description: idea.description, status: idea.status, priority: idea.priority}) });
    }
    loadProducts();
    loadProductDetail(productId);
  };

  // --- Manifests ---
  async function loadManifests() {
    const el = document.getElementById('manifest-list');
    const searchInput = document.getElementById('manifest-search-input');
    const searchBtn = document.getElementById('manifest-search-btn');

    searchBtn.onclick = () => searchManifests(searchInput.value);
    searchInput.onkeypress = (e) => { if (e.key === 'Enter') searchManifests(searchInput.value); };

    try {
      const [peerGroups, products] = await Promise.all([
        fetchJSON('/api/manifests/by-peer'),
        fetchJSON('/api/products'),
      ]);
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No manifests yet</div>';
        return;
      }

      // Build product lookup
      const prodMap = {};
      for (const p of (products || [])) {
        prodMap[p.id] = p;
      }

      // Peer → Product → Manifests (mirrors tasks: Peer → Manifest → Tasks)
      let html = '';
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];

        // Peer header (L1)
        html += `<div class="tree-node peer-header clickable" data-mfst-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-mfst-peer-children="${pi}">`;

        // Group manifests by product
        const byProduct = {};
        const productOrder = [];
        for (const m of pg.manifests) {
          const pid = (m.project_id && prodMap[m.project_id]) ? m.project_id : '__unassigned__';
          if (!byProduct[pid]) {
            byProduct[pid] = [];
            productOrder.push(pid);
          }
          byProduct[pid].push(m);
        }

        // Product headers (L2)
        for (let pri = 0; pri < productOrder.length; pri++) {
          const pid = productOrder[pri];
          const items = byProduct[pid];
          const prod = prodMap[pid];
          const prodTitle = prod ? prod.title : 'Unassigned';
          const prodMarker = prod ? prod.marker : '';
          const dotColor = prod ? 'green' : '';

          html += `<div class="tree-node clickable" style="padding-left:24px" data-mfst-prod="${pi}-${pri}">
            <span class="tree-arrow">&#x25B6;</span>
            ${dotColor ? `<span class="status-dot ${dotColor}"></span>` : ''}
            <span>${esc(prodTitle)}</span>
            ${prodMarker ? `<span class="session-uuid">${esc(prodMarker)}</span>` : ''}
            <span class="count">${items.length}</span>
          </div>`;
          html += `<div style="display:none" data-mfst-prod-children="${pi}-${pri}">`;

          // Manifest items (L3)
          for (const m of items) {
            const statusClass = m.status === 'open' ? 'confirmed' : m.status === 'closed' ? 'flagged' : 'dismissed';
            const metaParts = [];
            if (m.total_tasks > 0) metaParts.push(`${m.total_tasks} tasks`);
            if (m.total_turns > 0) metaParts.push(`${m.total_turns} turns`);
            if (m.total_cost > 0) metaParts.push(`$${m.total_cost.toFixed(2)}`);

            html += `<div class="amnesia-item ${statusClass} clickable" data-id="${esc(m.id)}" style="margin-left:24px;cursor:pointer">
              <div class="amnesia-header">
                <span class="amnesia-status-label">${esc(m.status)}</span>
                <span class="session-uuid">${esc(m.marker)}</span>
                ${metaParts.length ? `<span style="font-size:11px;color:var(--text-muted)">${metaParts.join(' &middot; ')}</span>` : ''}
                <span style="color:var(--text-muted);font-size:11px;margin-left:auto">${formatTime(m.updated_at || '')}</span>
              </div>
              <div class="amnesia-rule">${esc(m.title)}</div>
            </div>`;
          }
          html += `</div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer toggle (L1)
      el.querySelectorAll('[data-mfst-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.mfstPeer;
          const children = el.querySelector(`[data-mfst-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Product toggle (L2)
      el.querySelectorAll('[data-mfst-prod]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.mfstProd;
          const children = el.querySelector(`[data-mfst-prod-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Manifest clicks (L3)
      el.querySelectorAll('.amnesia-item.clickable').forEach(item => {
        item.addEventListener('click', (e) => {
          e.stopPropagation();
          window._loadManifest(item.dataset.id);
        });
      });
    } catch (e) {
      console.error('Load manifests failed:', e);
    }
  }

  async function searchManifests(query) {
    if (!query.trim()) { loadManifests(); return; }
    const el = document.getElementById('manifest-list');
    try {
      const resp = await fetch('/api/manifests/search', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({query})
      });
      const manifests = await resp.json();
      renderManifestList(el, manifests || []);
    } catch (e) {
      console.error('Search manifests failed:', e);
    }
  }

  function renderManifestList(el, manifests) {
    if (!manifests || !manifests.length) {
      el.innerHTML = '<div class="empty-state">No manifests found</div>';
      return;
    }
    el.innerHTML = manifests.map(m => {
      const statusClass = m.status === 'open' ? 'scope' : m.status === 'closed' ? 'type' : m.status === 'archive' ? 'type' : '';
      const jira = (m.jira_refs || []).join(', ');
      return `<div class="manifest-item clickable" data-id="${esc(m.id)}" onclick="window._loadManifest('${esc(m.id)}')">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:4px">
          <span class="session-uuid">${esc(m.marker)}</span>
          <span class="badge ${statusClass}">${esc(m.status)}</span>
          <span style="font-size:11px;color:var(--text-muted)">v${m.version}</span>
          <button class="btn-copy-sm" onclick="event.stopPropagation();window._copy('get manifest ${esc(m.marker)}')" title="Copy ref">&#x2398;</button>
        </div>
        <div class="manifest-item-title">${esc(m.title)}</div>
        <div style="font-size:12px;color:var(--text-secondary)">${esc(m.description)}</div>
        ${jira ? `<div style="font-size:11px;color:var(--accent);margin-top:4px">${esc(jira)}</div>` : ''}
      </div>`;
    }).join('');
  }

  // Promote idea or memory to manifest — opens manifest detail panel with creation form pre-filled
  window._promoteToManifest = async function(title, description, content) {
    switchView('manifests');

    // Fetch products for the dropdown
    let products = [];
    try {
      const groups = await fetchJSON('/api/products/by-peer');
      if (groups) {
        for (const g of groups) {
          for (const p of (g.products || [])) {
            products.push(p);
          }
        }
      }
    } catch(e) {}

    setTimeout(() => {
      const titleEl = document.getElementById('manifest-detail-title');
      const bodyEl = document.getElementById('manifest-detail');
      titleEl.textContent = 'New Manifest';

      const productOptions = products.map(p =>
        `<option value="${esc(p.id)}">${esc(p.marker)} ${esc(p.title)}</option>`
      ).join('');

      bodyEl.innerHTML = `
        <div style="max-width:600px">
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Title</label>
            <input type="text" id="pm-title" class="conv-search" style="font-size:14px" value="${esc(title)}" />
          </div>
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Description</label>
            <input type="text" id="pm-description" class="conv-search" style="font-size:13px" value="${esc(description)}" />
          </div>
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Content</label>
            <textarea id="pm-content" class="conv-search" style="font-size:13px;height:200px;resize:vertical;font-family:var(--font-mono)">${esc(content)}</textarea>
          </div>
          <div style="display:flex;gap:12px;margin-bottom:16px">
            <div style="flex:1">
              <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Status</label>
              <select id="pm-status" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
                <option value="draft" selected>Draft</option>
                <option value="open">Open</option>
                <option value="closed">Closed</option>
              </select>
            </div>
            <div style="flex:1">
              <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Product</label>
              <select id="pm-product-id" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
                <option value="">No Product</option>
                ${productOptions}
              </select>
            </div>
          </div>
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Jira Refs</label>
            <input type="text" id="pm-jira" class="conv-search" placeholder="ENG-1234, ENG-5678" style="font-size:13px" />
          </div>
          <div style="display:flex;gap:8px">
            <button id="pm-submit" class="btn-search" style="padding:8px 20px">Create Manifest</button>
            <span id="pm-status-msg" style="font-size:13px;color:var(--green);align-self:center"></span>
          </div>
        </div>
      `;

      bodyEl.querySelector('#pm-submit').onclick = async () => {
        const t = bodyEl.querySelector('#pm-title').value.trim();
        const d = bodyEl.querySelector('#pm-description').value.trim();
        const c = bodyEl.querySelector('#pm-content').value.trim();
        const s = bodyEl.querySelector('#pm-status').value;
        const pid = bodyEl.querySelector('#pm-product-id').value;
        const j = bodyEl.querySelector('#pm-jira').value.trim();
        if (!t) { bodyEl.querySelector('#pm-status-msg').textContent = 'Title required'; return; }

        const resp = await fetch('/api/manifests', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({title: t, description: d, content: c, status: s, project_id: pid, jira_refs: j ? j.split(',').map(s => s.trim()) : []})
        });
        const m = await resp.json();
        bodyEl.querySelector('#pm-status-msg').textContent = 'Created!';
        loadManifests();
        setTimeout(() => window._loadManifest(m.id), 500);
      };
    }, 300);
  };

  window._loadManifest = async function(id) {
    document.querySelectorAll('.manifest-item').forEach(i => i.classList.remove('active'));
    const active = document.querySelector(`.manifest-item[data-id="${id}"]`);
    if (active) active.classList.add('active');

    try {
      const m = await fetchJSON('/api/manifests/' + id);
      if (!m) return;

      const titleEl = document.getElementById('manifest-detail-title');
      const bodyEl = document.getElementById('manifest-detail');

      const jira = (m.jira_refs || []).map(r => `<a href="https://gryphonnetworks.atlassian.net/browse/${r}" target="_blank" style="color:var(--accent)">${esc(r)}</a>`).join(', ');
      const tags = (m.tags || []).map(t => `<span class="badge tag">${esc(t)}</span>`).join(' ');

      titleEl.innerHTML = `<span id="manifest-edit-title" class="manifest-editable" style="cursor:pointer;border-radius:4px;padding:2px 4px" title="Click to edit title">${esc(m.title)}</span> <button class="btn-copy" onclick="window._copy('get manifest ${esc(m.marker)}')" title="Copy ref">&#x2398;</button>`;

      // Fetch linked product for breadcrumb
      let product = null;
      if (m.project_id) {
        try { product = await fetchJSON('/api/products/' + m.project_id); } catch(e) {}
      }

      // Fetch all products for assignment dropdown
      let allProducts = [];
      try {
        const groups = await fetchJSON('/api/products/by-peer');
        if (groups) {
          for (const g of groups) {
            for (const p of (g.products || [])) {
              allProducts.push(p);
            }
          }
        }
      } catch(e) {}

      // Fetch linked ideas
      let linkedIdeasHtml = '';
      try {
        const linkedIdeas = await fetchJSON('/api/manifests/' + m.id + '/ideas');
        if (linkedIdeas && linkedIdeas.length) {
          linkedIdeasHtml = '<div style="margin-bottom:12px"><span style="color:var(--text-muted);font-size:12px">Ideas:</span> ' +
            linkedIdeas.map(i => `<span class="badge idea-nav" style="cursor:pointer;color:var(--green)" data-iid="${i.marker}">${esc(i.marker)} ${esc(i.title)}</span>`).join(' ') +
            '</div>';
        }
      } catch(e) {}

      // Fetch linked tasks with turns/cost
      let linkedTasksHtml = '';
      try {
        const linkedTasks = await fetchJSON('/api/manifests/' + m.id + '/tasks');
        if (linkedTasks && linkedTasks.length) {
          const statusCounts = {};
          for (const t of linkedTasks) { statusCounts[t.status] = (statusCounts[t.status] || 0) + 1; }
          const summary = Object.entries(statusCounts).map(([s,c]) => `${c} ${s}`).join(', ');
          const statusColors = {running:'var(--green)',paused:'var(--yellow)',scheduled:'var(--yellow)',waiting:'var(--accent)',pending:'var(--text-muted)',completed:'var(--green)',failed:'var(--red)',cancelled:'var(--text-muted)'};
          const statusIcons = {running:'&#x25CF;',paused:'&#x23F8;',scheduled:'&#x23F0;',waiting:'&#x23F3;',pending:'&#x25CB;',completed:'&#x2713;',failed:'&#x2717;',cancelled:'&#x2015;'};
          const taskRows = linkedTasks.map(t => {
            const color = statusColors[t.status] || 'var(--text-muted)';
            const icon = statusIcons[t.status] || '&#x25CB;';
            const turnsStr = (t.total_turns || t.turns || 0) > 0 ? `${t.total_turns || t.turns} turns` : '';
            const costStr = (t.total_cost || t.cost || 0) > 0 ? `$${(t.total_cost || t.cost).toFixed(2)}` : '';
            const runsStr = t.run_count > 0 ? `${t.run_count} runs` : '';
            const lastRunStr = t.last_run_at ? `last: ${new Date(t.last_run_at).toLocaleString()}` : '';
            return `<div class="task-nav" data-tid="${t.id}" style="display:flex;align-items:center;gap:8px;padding:8px 10px;border-bottom:1px solid var(--border);cursor:pointer;font-size:12px">
              <span style="color:${color};font-size:14px;min-width:16px">${icon}</span>
              <span class="session-uuid" style="font-size:11px">${esc(t.marker)}</span>
              <span style="flex:1;color:var(--text-primary)">${esc(t.title)}</span>
              <span style="color:${color};font-weight:600;text-transform:uppercase;font-size:10px;min-width:60px">${esc(t.status)}</span>
              ${runsStr ? `<span style="color:var(--text-muted);font-size:11px;min-width:45px">${runsStr}</span>` : '<span style="min-width:45px"></span>'}
              ${turnsStr ? `<span style="color:var(--text-muted);font-size:11px;min-width:55px">${turnsStr}</span>` : '<span style="min-width:55px"></span>'}
              ${costStr ? `<span style="color:var(--text-muted);font-size:11px;min-width:45px">${costStr}</span>` : '<span style="min-width:45px"></span>'}
              ${lastRunStr ? `<span style="color:var(--text-muted);font-size:10px">${lastRunStr}</span>` : ''}
            </div>`;
          }).join('');
          linkedTasksHtml = `<div style="margin-bottom:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="font-size:13px;color:var(--text-primary);margin-bottom:8px;font-weight:600">Executed Tasks <span style="font-weight:400;font-size:12px;color:var(--text-muted)">(${linkedTasks.length}) &mdash; ${summary}</span></div>
            <div style="border:1px solid var(--border);border-radius:4px;overflow:hidden">${taskRows}</div>
          </div>`;
        } else {
          linkedTasksHtml = `<div style="margin-bottom:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="font-size:13px;color:var(--text-primary);margin-bottom:8px;font-weight:600">Executed Tasks</div>
            <div style="font-size:12px;color:var(--text-muted);padding:12px;border:1px dashed var(--border);border-radius:4px;text-align:center">
              No tasks executed yet
              <button class="btn-search manifest-create-task-btn" style="margin-left:8px;padding:4px 12px;font-size:11px">+ Create Task</button>
            </div>
          </div>`;
        }
      } catch(e) {}

      const statusOptions = ['draft', 'open', 'closed', 'archive'];
      const statusBtnColors = {draft:'var(--yellow)',open:'var(--green)',closed:'var(--text-muted)',archive:'var(--red)'};
      const statusToggleHtml = statusOptions.map(s => {
        const active = m.status === s;
        const color = statusBtnColors[s] || 'var(--text-muted)';
        return `<button class="manifest-status-btn" data-status="${s}" style="
          padding:4px 12px;font-size:11px;font-weight:600;text-transform:uppercase;border-radius:4px;cursor:pointer;
          border:1px solid ${color};
          background:${active ? color : 'transparent'};
          color:${active ? 'var(--bg-primary)' : color};
          opacity:${active ? '1' : '0.7'};
        ">${s}</button>`;
      }).join('');

      bodyEl.innerHTML = `
        <div class="manifest-detail-view">
          <!-- BREADCRUMB -->
          <div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;font-family:var(--font-mono)">
            <span style="cursor:pointer;color:var(--accent)" onclick="switchView('${product ? 'products' : 'manifests'}')">${esc(m.source_node ? m.source_node.substring(0,12) : 'node')}</span>
            ${product ? `<span style="opacity:0.4"> → </span><span style="cursor:pointer;color:var(--accent)" onclick="switchView('products');setTimeout(()=>window._loadProduct('${esc(product.id)}'),300)">${esc(product.marker)} ${esc(product.title)}</span>` : ''}
            <span style="opacity:0.4"> → </span>
            <span style="color:var(--text-primary)">${esc(m.marker)} ${esc(m.title)}</span>
          </div>
          <div class="manifest-meta">
            <span class="session-uuid" style="font-size:14px">${esc(m.marker)}</span>
            <div style="display:inline-flex;gap:4px;margin:0 8px">${statusToggleHtml}</div>
            <span style="font-size:12px;color:var(--text-muted)">v${m.version}</span>
            <span style="font-size:12px;color:var(--text-muted)">by ${esc(m.author)}</span>
          </div>
          <!-- METADATA BAR -->
          <div style="display:flex;gap:12px;font-size:12px;color:var(--text-muted);margin-bottom:12px;align-items:center;flex-wrap:wrap;padding:8px 12px;background:var(--bg-secondary);border:1px solid var(--border);border-radius:6px;font-family:var(--font-mono)">
            <span>Tasks: <strong style="color:var(--text-primary)">${m.total_tasks || 0}</strong></span>
            <span>Turns: <strong style="color:var(--text-primary)">${m.total_turns || 0}</strong></span>
            <span>Cost: <strong style="color:var(--green)">$${(m.total_cost || 0).toFixed(2)}</strong></span>
            <span style="opacity:0.3">|</span>
            <span>Product: <strong id="manifest-product-display" style="color:${product ? 'var(--accent)' : 'var(--text-muted)'};cursor:pointer" title="Click to change product">${product ? esc(product.marker) + ' ' + esc(product.title) : 'None'}</strong></span>
            <span style="opacity:0.3">|</span>
            <span>Created: ${new Date(m.created_at).toLocaleString()}</span>
            <span>Updated: ${new Date(m.updated_at).toLocaleString()}</span>
          </div>
          <div id="manifest-edit-desc" class="manifest-editable" style="font-size:13px;color:var(--text-secondary);margin-bottom:12px;padding:4px;border-radius:4px;cursor:pointer" title="Click to edit description">${esc(m.description) || '<span style="color:var(--text-muted);font-style:italic">No description — click to add</span>'}</div>
          ${linkedIdeasHtml}
          ${linkedTasksHtml}
          ${jira ? `<div style="margin-bottom:12px">Jira: ${jira}</div>` : ''}
          ${tags ? `<div style="margin-bottom:12px">${tags}</div>` : ''}
          <div style="margin-bottom:4px;display:flex;align-items:center;gap:8px">
            <span style="font-size:12px;color:var(--text-muted);font-weight:500">Spec / Content</span>
            <button id="manifest-edit-content-btn" class="btn-search" style="padding:2px 10px;font-size:11px">Edit</button>
          </div>
          <div id="manifest-content-display" class="manifest-content">${esc(m.content)}</div>
          <div id="manifest-content-editor" style="display:none;margin-bottom:12px">
            <textarea id="manifest-content-textarea" class="conv-search" style="width:100%;min-height:300px;font-family:monospace;font-size:13px;padding:12px;resize:vertical;line-height:1.5">${esc(m.content)}</textarea>
            <div style="margin-top:6px;display:flex;gap:8px">
              <button id="manifest-content-save" class="btn-search" style="padding:4px 16px;font-size:12px">Save</button>
              <button id="manifest-content-cancel" class="btn-dismiss" style="padding:4px 12px;font-size:12px">Cancel</button>
            </div>
          </div>
          <div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border);font-size:11px;color:var(--text-muted);display:flex;gap:16px">
            <span>Created: ${new Date(m.created_at).toLocaleString()}</span>
            <span>Updated: ${new Date(m.updated_at).toLocaleString()}</span>
            <span>ID: ${esc(m.id)}</span>
          </div>
        </div>`;

      // Bind idea navigation links
      bodyEl.querySelectorAll('.idea-nav').forEach(el => {
        el.addEventListener('click', () => window._goToIdea(el.dataset.iid));
      });

      // Bind task navigation links
      bodyEl.querySelectorAll('.task-nav').forEach(el => {
        el.addEventListener('click', () => {
          switchView('tasks');
          setTimeout(() => loadTaskDetail(el.dataset.tid), 300);
        });
      });

      // Bind status toggle buttons
      bodyEl.querySelectorAll('.manifest-status-btn').forEach(btn => {
        btn.addEventListener('click', async () => {
          const newStatus = btn.dataset.status;
          if (newStatus === m.status) return;
          try {
            await fetch('/api/manifests/' + m.id, {
              method: 'PUT',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({status: newStatus})
            });
            window._loadManifest(m.id);
            loadManifests();
          } catch(e) {
            console.error('Update manifest status failed:', e);
          }
        });
      });

      // Inline edit: Title (click to edit)
      const titleSpan = document.getElementById('manifest-edit-title');
      if (titleSpan) {
        titleSpan.addEventListener('click', () => {
          const input = document.createElement('input');
          input.type = 'text';
          input.value = m.title;
          input.className = 'conv-search';
          input.style.cssText = 'font-size:inherit;font-weight:inherit;padding:2px 4px;width:300px';
          titleSpan.replaceWith(input);
          input.focus();
          input.select();
          const save = async () => {
            const val = input.value.trim();
            if (val && val !== m.title) {
              await fetch('/api/manifests/' + m.id, {
                method: 'PUT',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({title: val})
              });
              loadManifests();
            }
            window._loadManifest(m.id);
          };
          input.addEventListener('blur', save);
          input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); save(); } if (e.key === 'Escape') window._loadManifest(m.id); });
        });
      }

      // Inline edit: Description (click to edit)
      const descEl = document.getElementById('manifest-edit-desc');
      if (descEl) {
        descEl.addEventListener('click', () => {
          const input = document.createElement('input');
          input.type = 'text';
          input.value = m.description || '';
          input.className = 'conv-search';
          input.style.cssText = 'font-size:13px;padding:4px;width:100%';
          input.placeholder = 'Enter description...';
          descEl.replaceWith(input);
          input.focus();
          const save = async () => {
            const val = input.value.trim();
            if (val !== (m.description || '')) {
              await fetch('/api/manifests/' + m.id, {
                method: 'PUT',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({description: val})
              });
              loadManifests();
            }
            window._loadManifest(m.id);
          };
          input.addEventListener('blur', save);
          input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); save(); } if (e.key === 'Escape') window._loadManifest(m.id); });
        });
      }

      // Inline edit: Product assignment (click to show dropdown)
      const productDisplay = document.getElementById('manifest-product-display');
      if (productDisplay) {
        productDisplay.addEventListener('click', () => {
          const productOpts = allProducts.map(p =>
            `<option value="${esc(p.id)}"${m.project_id === p.id ? ' selected' : ''}>${esc(p.marker)} ${esc(p.title)}</option>`
          ).join('');
          const sel = document.createElement('select');
          sel.className = 'conv-filter';
          sel.style.cssText = 'font-size:12px;padding:2px 6px;font-family:var(--font-mono)';
          sel.innerHTML = `<option value="">No Product</option>${productOpts}`;
          sel.value = m.project_id || '';
          productDisplay.replaceWith(sel);
          sel.focus();
          const save = async () => {
            const newPid = sel.value;
            if (newPid !== (m.project_id || '')) {
              await fetch('/api/manifests/' + m.id, {
                method: 'PUT',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({project_id: newPid})
              });
              loadManifests();
            }
            window._loadManifest(m.id);
          };
          sel.addEventListener('change', save);
          sel.addEventListener('blur', () => window._loadManifest(m.id));
        });
      }

      // Inline edit: Content (toggle editor)
      const contentBtn = document.getElementById('manifest-edit-content-btn');
      const contentDisplay = document.getElementById('manifest-content-display');
      const contentEditor = document.getElementById('manifest-content-editor');
      if (contentBtn && contentDisplay && contentEditor) {
        contentBtn.addEventListener('click', () => {
          contentDisplay.style.display = 'none';
          contentBtn.style.display = 'none';
          contentEditor.style.display = 'block';
        });
        document.getElementById('manifest-content-cancel').addEventListener('click', () => {
          contentEditor.style.display = 'none';
          contentDisplay.style.display = '';
          contentBtn.style.display = '';
        });
        document.getElementById('manifest-content-save').addEventListener('click', async () => {
          const val = document.getElementById('manifest-content-textarea').value;
          await fetch('/api/manifests/' + m.id, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({content: val})
          });
          loadManifests();
          window._loadManifest(m.id);
        });
      }

      // Create task button (when no tasks exist for this manifest)
      const createTaskBtn = bodyEl.querySelector('.manifest-create-task-btn');
      if (createTaskBtn) {
        createTaskBtn.addEventListener('click', () => {
          switchView('tasks');
          setTimeout(() => {
            showTaskCreateForm();
            setTimeout(() => {
              const sel = document.getElementById('tc-manifest-id');
              if (sel) { sel.value = m.id; }
            }, 100);
          }, 300);
        });
      }

    } catch (e) {
      console.error('Load manifest failed:', e);
    }
  };

  // --- Ideas ---
  let _pendingIdeaId = null;

  async function loadIdeas() {
    const el = document.getElementById('ideas-list');
    const btn = document.getElementById('idea-add-btn');
    const titleInput = document.getElementById('idea-title');
    const prioritySelect = document.getElementById('idea-priority');

    btn.onclick = async () => {
      const title = titleInput.value.trim();
      if (!title) return;
      await fetch('/api/ideas', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({title, priority: prioritySelect.value})
      });
      titleInput.value = '';
      loadIdeas();
    };
    titleInput.onkeypress = (e) => { if (e.key === 'Enter') btn.click(); };

    try {
      const peerGroups = await fetchJSON('/api/ideas/by-peer');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state" style="padding:16px">No ideas yet</div>';
        return;
      }
      let html = '';
      let allIdeas = [];
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];
        html += `<div class="tree-node peer-header clickable" data-idea-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-idea-peer-children="${pi}">`;
        for (const i of pg.ideas) {
          allIdeas.push(i);
          const prioClass = i.priority === 'critical' || i.priority === 'high' ? 'high' : i.priority === 'low' ? 'low' : 'medium';
          html += `<div class="manifest-item clickable" data-id="${esc(i.id)}">
            <div style="display:flex;align-items:center;gap:6px;margin-bottom:4px">
              <span class="amnesia-score ${prioClass}" style="font-size:10px">${esc(i.priority)}</span>
              <span class="session-uuid">${esc(i.marker)}</span>
              <span class="badge">${esc(i.status)}</span>
            </div>
            <div class="manifest-item-title">${esc(i.title)}</div>
          </div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer toggle
      el.querySelectorAll('[data-idea-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.ideaPeer;
          const children = el.querySelector(`[data-idea-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      el.querySelectorAll('.manifest-item').forEach(item => {
        item.addEventListener('click', () => window._loadIdea(item.dataset.id));
      });

      if (_pendingIdeaId) {
        const id = _pendingIdeaId;
        _pendingIdeaId = null;
        const match = allIdeas.find(i => i.id.startsWith(id) || i.marker === id);
        if (match) window._loadIdea(match.id);
      }
    } catch (e) {
      console.error('Load ideas failed:', e);
    }
  }

  window._loadIdea = async function(id) {
    document.querySelectorAll('#ideas-list .manifest-item').forEach(i => i.classList.remove('active'));
    const active = document.querySelector(`#ideas-list .manifest-item[data-id="${id}"]`);
    if (active) active.classList.add('active');

    try {
      const idea = await fetchJSON('/api/ideas/' + id);
      if (!idea) return;

      const titleEl = document.getElementById('idea-detail-title');
      const bodyEl = document.getElementById('idea-detail');
      const prioClass = idea.priority === 'critical' || idea.priority === 'high' ? 'high' : idea.priority === 'low' ? 'low' : 'medium';

      titleEl.textContent = idea.title;

      // Fetch linked manifests
      let linkedHtml = '';
      try {
        const linked = await fetchJSON('/api/ideas/' + idea.id + '/manifests');
        if (linked && linked.length) {
          linkedHtml = '<div style="margin-bottom:12px"><span style="color:var(--text-muted);font-size:12px">Manifests:</span> ' +
            linked.map(m => `<span class="badge type manifest-link" style="cursor:pointer" data-mid="${m.id}">${esc(m.marker)} ${esc(m.title)}</span>`).join(' ') +
            '</div>';
        }
      } catch(e) {}

      bodyEl.innerHTML = `
        <div class="manifest-detail-view">
          <!-- BREADCRUMB -->
          <div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;font-family:var(--font-mono)">
            <span style="cursor:pointer;color:var(--accent)" onclick="switchView('ideas')">${esc(idea.source_node ? idea.source_node.substring(0,12) : 'node')}</span>
            <span style="opacity:0.4"> &rarr; </span>
            <span style="color:var(--text-primary)">${esc(idea.marker)} ${esc(idea.title)}</span>
          </div>
          <div class="manifest-meta">
            <span class="session-uuid" style="font-size:14px">${esc(idea.marker)}</span>
            <span class="amnesia-score ${prioClass}">${esc(idea.priority)}</span>
            <span class="badge">${esc(idea.status)}</span>
            <span style="font-size:12px;color:var(--text-muted)">by ${esc(idea.author)}</span>
            <button class="btn-copy" onclick="window._copy('get idea ${idea.marker}')" title="Copy ref">&#x2398;</button>
          </div>
          ${idea.description ? `<div style="font-size:14px;color:var(--text-primary);margin:12px 0;line-height:1.5">${esc(idea.description)}</div>` : ''}
          ${linkedHtml}
          <div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border);font-size:11px;color:var(--text-muted)">
            Created: ${new Date(idea.created_at).toLocaleString()} | ID: ${esc(idea.id)}
          </div>
          <div style="margin-top:12px;display:flex;gap:8px">
            <button class="btn-search promote-idea-btn" style="font-size:12px;padding:6px 14px">Create Manifest from Idea</button>
            <button class="btn-dismiss" onclick="window._archiveIdea('${esc(idea.id)}')">Archive</button>
          </div>
        </div>`;

      // Bind manifest links — click to navigate to manifest
      bodyEl.querySelectorAll('.manifest-link').forEach(el => {
        el.addEventListener('click', () => {
          switchView('manifests');
          setTimeout(() => window._loadManifest(el.dataset.mid), 300);
        });
      });

      // Promote idea to manifest
      bodyEl.querySelector('.promote-idea-btn').addEventListener('click', () => {
        window._promoteToManifest(
          idea.title,
          idea.description || '',
          `# ${idea.title}\n\n${idea.description || ''}\n\nPromoted from idea [${idea.marker}]\nPriority: ${idea.priority}\nStatus: ${idea.status}`
        );
      });
    } catch(e) {
      console.error('Load idea failed:', e);
    }
  };

  // Navigate from manifest → specific idea
  window._goToIdea = function(marker) {
    _pendingIdeaId = marker;
    switchView('ideas');
  };

  window._archiveManifest = async function(id) {
    await fetchJSON('/api/manifests/' + id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({status: 'archive'}) });
    loadManifests();
    window._loadManifest(id);
  };

  window._archiveMemory = async function(id) {
    // Memories don't have a status field — soft delete via API
    await fetch('/api/memories/' + id, {method: 'DELETE'});
    document.getElementById('memory-peer-detail').innerHTML = '<div class="empty-state">Archived</div>';
    loadMemoryPeerTree();
  };

  window._archiveIdea = async function(id) {
    const idea = await fetchJSON('/api/ideas/' + id);
    if (idea) {
      await fetchJSON('/api/ideas/' + id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({status: 'archive'}) });
    }
    loadIdeas();
  };

  // --- Parse Task Output ---
  function parseTaskOutput(raw) {
    if (!raw) return '<div class="empty-state">No output</div>';
    let lines = raw.split('\n').filter(l => l.trim());
    const totalLines = lines.length;
    // For large outputs, only parse — keep all lines but cap display
    if (lines.length > 500) {
      lines = lines.slice(-500);
    }
    let html = '';
    let turnNum = 0;
    let resultInfo = null;

    for (const line of lines) {
      try {
        const event = JSON.parse(line);
        if (event.type === 'assistant' && event.message) {
          turnNum++;
          const content = event.message.content || [];
          for (const block of content) {
            if (block.type === 'text' && block.text) {
              const text = block.text.length > 500 ? block.text.substring(0, 500) + '...' : block.text;
              html += `<div style="padding:6px 0;border-bottom:1px solid var(--border);font-size:12px">
                <span style="color:var(--green);font-weight:500">#${turnNum}</span>
                <span style="color:var(--text-primary)">${esc(text)}</span>
              </div>`;
            }
            if (block.type === 'tool_use') {
              const name = block.name || '?';
              let inputPreview = '';
              if (block.input) {
                if (block.input.command) inputPreview = block.input.command;
                else if (block.input.file_path) inputPreview = block.input.file_path;
                else if (block.input.pattern) inputPreview = block.input.pattern;
                else if (block.input.query) inputPreview = block.input.query;
                else inputPreview = JSON.stringify(block.input).substring(0, 100);
              }
              if (inputPreview.length > 80) inputPreview = inputPreview.substring(0, 80) + '...';
              html += `<div style="padding:4px 0;border-bottom:1px solid var(--border);font-size:12px">
                <span class="badge type" style="font-size:10px">${esc(name)}</span>
                <span style="font-family:var(--font-mono);font-size:11px;color:var(--text-secondary)">${esc(inputPreview)}</span>
              </div>`;
            }
          }
        }
        if (event.type === 'result') {
          resultInfo = event;
        }
      } catch (e) {}
    }

    if (resultInfo) {
      const reason = resultInfo.terminal_reason || resultInfo.stop_reason || '?';
      const turns = resultInfo.num_turns || '?';
      const cost = resultInfo.total_cost_usd ? '$' + resultInfo.total_cost_usd.toFixed(2) : '?';
      const reasonColor = reason === 'completed' ? 'var(--green)' : reason === 'max_turns' ? 'var(--yellow)' : 'var(--red)';
      html += `<div style="padding:8px 0;font-size:12px;font-weight:500;border-top:2px solid var(--border);margin-top:4px">
        <span style="color:${reasonColor}">${esc(reason)}</span>
        <span style="color:var(--text-muted);margin-left:12px">${turns} turns</span>
        <span style="color:var(--text-muted);margin-left:12px">${cost}</span>
      </div>`;
    }

    return html || '<div style="font-size:12px;color:var(--text-muted)">No parseable output</div>';
  }

  // --- Tasks ---
  async function loadTasks() {
    const el = document.getElementById('tasks-list');
    const newBtn = document.getElementById('task-new-btn');

    newBtn.onclick = () => showTaskCreateForm();

    const statusColors = {running:'green',paused:'yellow',scheduled:'yellow',waiting:'yellow',pending:'',completed:'green',max_turns:'yellow',failed:'red',cancelled:''};

    try {
      const peerGroups = await fetchJSON('/api/tasks/by-peer');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state" style="padding:16px">No tasks yet</div>';
        return;
      }

      // Peer → Manifest → Tasks (mirrors amnesia: Peer → Session → Violations)
      let html = '';
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];
        const totalTasks = pg.manifests.reduce((sum, mg) => sum + mg.tasks.length, 0);

        // Peer header (L1)
        html += `<div class="tree-node peer-header clickable" data-task-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${totalTasks}</span>
        </div>`;
        html += `<div class="peer-children" data-task-peer-children="${pi}">`;

        // Manifest headers (L2) — same as amnesia sessions
        for (let mi = 0; mi < pg.manifests.length; mi++) {
          const mg = pg.manifests[mi];
          if (!mg.tasks.length) continue;

          // Pick dot color from most active task status
          const hasRunning = mg.tasks.some(t => t.status === 'running');
          const hasFailed = mg.tasks.some(t => t.status === 'failed');
          const hasScheduled = mg.tasks.some(t => t.status === 'scheduled' || t.status === 'waiting');
          const dotColor = hasRunning ? 'green' : hasFailed ? 'red' : hasScheduled ? 'yellow' : 'green';

          html += `<div class="tree-node clickable" style="padding-left:24px" data-task-manifest="${pi}-${mi}">
            <span class="tree-arrow">&#x25B6;</span>
            <span class="status-dot ${dotColor}"></span>
            <span>${esc(mg.manifest_title || 'Standalone')}</span>
            <span class="session-uuid">${esc(mg.manifest_marker || '')}</span>
            <span class="count">${mg.tasks.length}</span>
          </div>`;
          html += `<div style="display:none" data-task-manifest-children="${pi}-${mi}">`;

          // Task items (L3) — same as amnesia violations
          for (const t of mg.tasks) {
            const sColor = statusColors[t.status] || '';
            const statusClass = t.status === 'completed' ? 'confirmed' : t.status === 'failed' ? 'flagged' : t.status === 'running' ? 'confirmed' : 'dismissed';
            const metaParts = [];
            if (t.run_count > 0) metaParts.push(`${t.run_count} runs`);
            if (t.total_turns > 0) metaParts.push(`${t.total_turns} turns`);
            if (t.total_cost > 0) metaParts.push(`$${t.total_cost.toFixed(2)}`);
            metaParts.push(t.schedule);

            html += `<div class="amnesia-item ${statusClass} clickable" data-id="${esc(t.id)}" style="margin-left:24px;cursor:pointer">
              <div class="amnesia-header">
                ${t.status === 'running' ? '<span class="status-dot green status-pulse"></span>' : `<span class="status-dot ${sColor}"></span>`}
                <span class="amnesia-status-label">${esc(t.status)}</span>
                <span class="session-uuid">${esc(t.marker)}</span>
                <span class="badge type">${esc(t.schedule)}</span>
                ${t.depends_on ? '<span class="badge scope">dep</span>' : ''}
                <span style="color:var(--text-muted);font-size:11px;margin-left:auto">${formatTime(t.updated_at || t.created_at)}</span>
              </div>
              <div class="amnesia-rule">${esc(t.title)}</div>
              <div class="amnesia-action">
                ${metaParts.join(' &middot; ')}
              </div>
            </div>`;
          }
          html += `</div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer toggle (L1)
      el.querySelectorAll('[data-task-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.taskPeer;
          const children = el.querySelector(`[data-task-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Manifest toggle (L2)
      el.querySelectorAll('[data-task-manifest]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.taskManifest;
          const children = el.querySelector(`[data-task-manifest-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Task clicks (L3)
      el.querySelectorAll('.amnesia-item.clickable').forEach(item => {
        item.addEventListener('click', (e) => {
          e.stopPropagation();
          el.querySelectorAll('.amnesia-item').forEach(i => i.classList.remove('active'));
          item.classList.add('active');
          loadTaskDetail(item.dataset.id);
        });
      });
    } catch (e) {
      console.error('Load tasks failed:', e);
    }
  }

  async function loadTaskDetail(id) {
    try {
      const t = await fetchJSON('/api/tasks/' + id);
      if (!t) return;
      const titleEl = document.getElementById('task-detail-title');
      const bodyEl = document.getElementById('task-detail');

      const statusColor = t.status === 'running' ? 'var(--green)' : t.status === 'paused' ? 'var(--yellow)' : t.status === 'scheduled' ? 'var(--yellow)' : t.status === 'failed' ? 'var(--red)' : 'var(--text-muted)';

      // Fetch manifest + product for breadcrumb
      let taskManifest = null, taskProduct = null;
      if (t.manifest_id) {
        try {
          taskManifest = await fetchJSON('/api/manifests/' + t.manifest_id);
          if (taskManifest && taskManifest.project_id) {
            taskProduct = await fetchJSON('/api/products/' + taskManifest.project_id);
          }
        } catch(e) {}
      }

      // Fetch linked actions, amnesia, delusions, and run history in parallel
      let actionsHtml = '', amnesiaHtml = '', delusionsHtml = '', runsHtml = '';
      try {
        const [actions, amnesia, delusions, runs] = await Promise.all([
          fetchJSON('/api/tasks/' + t.id + '/actions'),
          fetchJSON('/api/tasks/' + t.id + '/amnesia'),
          fetchJSON('/api/tasks/' + t.id + '/delusions'),
          fetchJSON('/api/tasks/' + t.id + '/runs'),
        ]);
        if (actions && actions.length) {
          actionsHtml = `<div style="margin-bottom:12px">
            <div style="font-size:12px;color:var(--text-muted);margin-bottom:6px;font-weight:500">Actions (${actions.length})</div>
            <div style="display:flex;flex-direction:column;gap:2px">
              ${actions.slice(0, 50).map((a, i) => {
                const inputPreview = a.tool_input ? (a.tool_input.length > 80 ? a.tool_input.substring(0, 80) + '...' : a.tool_input) : '';
                const hasResponse = a.tool_response && a.tool_response.length > 0;
                return `<div class="task-action-item" style="border:1px solid var(--border);border-radius:4px;padding:6px 8px;font-size:11px">
                  <div style="display:flex;align-items:center;gap:6px;cursor:pointer" onclick="this.parentElement.querySelector('.task-action-detail').style.display=this.parentElement.querySelector('.task-action-detail').style.display==='none'?'':'none'">
                    <span style="color:var(--text-muted);font-size:10px;min-width:16px">#${actions.length - i}</span>
                    <span class="badge type" style="font-size:10px">${esc(a.tool_name)}</span>
                    ${hasResponse ? '<span style="color:var(--green);font-size:9px">&#x2713;</span>' : '<span style="color:var(--yellow);font-size:9px">&#x25CB;</span>'}
                    <span style="color:var(--text-muted);font-size:10px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1">${esc(inputPreview)}</span>
                    <span style="font-size:9px;color:var(--text-muted)">&#x25BC;</span>
                  </div>
                  <div class="task-action-detail" style="display:none;margin-top:6px">
                    ${a.tool_input ? `<div style="margin-bottom:4px"><span style="font-size:10px;color:var(--text-muted);font-weight:500">Input:</span><pre class="action-response" style="margin:2px 0;max-height:150px;overflow-y:auto;font-size:10px">${esc(a.tool_input)}</pre></div>` : ''}
                    ${hasResponse ? `<div><span style="font-size:10px;color:var(--text-muted);font-weight:500">Response:</span><pre class="action-response" style="margin:2px 0;max-height:150px;overflow-y:auto;font-size:10px">${esc(a.tool_response)}</pre></div>` : ''}
                  </div>
                </div>`;
              }).join('')}
            </div>
          </div>`;
        }
        if (amnesia && amnesia.length) {
          amnesiaHtml = `<div style="margin-bottom:12px;border-left:3px solid var(--red);padding-left:10px">
            <div style="font-size:12px;color:var(--red);margin-bottom:6px;font-weight:500">Visceral Violations (${amnesia.length})</div>
            ${amnesia.map(a => `<div style="font-size:12px;margin-bottom:4px">
              <span class="amnesia-score high" style="font-size:10px">${Math.round(a.score*100)}%</span>
              <span style="color:var(--text-secondary)">Rule [${esc(a.rule_marker)}]:</span> ${esc(a.rule_text.length > 60 ? a.rule_text.substring(0,60) + '...' : a.rule_text)}
            </div>`).join('')}
          </div>`;
        }
        if (delusions && delusions.length) {
          delusionsHtml = `<div style="margin-bottom:12px;border-left:3px solid var(--yellow);padding-left:10px">
            <div style="font-size:12px;color:var(--yellow);margin-bottom:6px;font-weight:500">Manifest Deviations (${delusions.length})</div>
            ${delusions.map(d => `<div style="font-size:12px;margin-bottom:4px">
              <span class="amnesia-score medium" style="font-size:10px">${Math.round(d.score*100)}%</span>
              <span style="color:var(--text-secondary)">${esc(d.reason.length > 80 ? d.reason.substring(0,80) + '...' : d.reason)}</span>
            </div>`).join('')}
          </div>`;
        }
        if (runs && runs.length) {
          const totalTaskCost = runs.reduce((s, r) => s + (r.cost_usd || 0), 0);
          const totalTaskTurns = runs.reduce((s, r) => s + (r.turns || 0), 0);
          runsHtml = `<div style="margin-bottom:12px">
            <div style="display:flex;align-items:center;gap:12px;margin-bottom:6px">
              <span style="font-size:12px;color:var(--text-muted);font-weight:500">Run History (${runs.length})</span>
              ${totalTaskCost > 0 ? `<span style="font-size:11px;color:var(--green);font-weight:600">Total: $${totalTaskCost.toFixed(2)}</span>` : ''}
              ${totalTaskTurns > 0 ? `<span style="font-size:11px;color:var(--text-muted)">${totalTaskTurns} turns</span>` : ''}
            </div>
            <div style="display:flex;flex-direction:column;gap:2px">
              ${runs.map(r => {
                const statusColor = r.status === 'completed' ? 'var(--green)' : r.status === 'failed' ? 'var(--red)' : 'var(--yellow)';
                const duration = r.completed_at && r.started_at ? formatDuration(new Date(r.completed_at) - new Date(r.started_at)) : '';
                const runCost = r.cost_usd ? `$${r.cost_usd.toFixed(2)}` : '';
                const runTurns = r.turns ? `${r.turns}t` : '';
                return `<div class="task-run-item" style="border:1px solid var(--border);border-radius:4px;padding:6px 8px;font-size:11px;cursor:pointer" data-run-id="${r.id}" data-task-id="${esc(t.id)}">
                  <div style="display:flex;align-items:center;gap:8px">
                    <span style="font-weight:600;min-width:24px">#${r.run_number}</span>
                    <span style="color:${statusColor};font-weight:600;text-transform:uppercase;font-size:10px">${esc(r.status)}</span>
                    <span style="color:var(--text-muted);font-size:10px">${r.actions} actions</span>
                    <span style="color:var(--text-muted);font-size:10px">${r.lines} lines</span>
                    ${runTurns ? `<span style="color:var(--text-muted);font-size:10px">${runTurns}</span>` : ''}
                    ${runCost ? `<span style="color:var(--green);font-size:10px;font-weight:600">${runCost}</span>` : ''}
                    ${duration ? `<span style="color:var(--text-muted);font-size:10px">${duration}</span>` : ''}
                    <span style="color:var(--text-muted);font-size:10px;margin-left:auto">${new Date(r.started_at).toLocaleString()}</span>
                  </div>
                </div>`;
              }).join('')}
            </div>
          </div>`;
        }
      } catch(e) {}

      const scheduleDisplay = t.schedule.startsWith('at:') ? new Date(t.schedule.substring(3)).toLocaleString() : t.schedule;

      // Determine one-shot vs recurring
      const isOneShot = t.schedule === 'once' || t.schedule === '' || t.schedule.startsWith('at:') || t.schedule.startsWith('in:');
      const isRunningOrPaused = t.status === 'running' || t.status === 'paused';

      // Determine current schedule select value
      let scheduleSelectVal = t.schedule;
      if (t.schedule.startsWith('at:')) scheduleSelectVal = 'at';
      if (!['once','in:5m','in:15m','in:30m','in:1h','at','5m','15m','30m','1h','6h','24h'].includes(scheduleSelectVal)) {
        scheduleSelectVal = isOneShot ? 'once' : t.schedule;
      }

      titleEl.textContent = t.title;
      bodyEl.innerHTML = `
        <div>
          <!-- BREADCRUMB -->
          <div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;font-family:var(--font-mono)">
            <span style="cursor:pointer;color:var(--accent)" onclick="switchView('${taskProduct ? 'products' : 'tasks'}')">${esc(t.source_node ? t.source_node.substring(0,12) : 'node')}</span>
            ${taskProduct ? `<span style="opacity:0.4"> → </span><span style="cursor:pointer;color:var(--accent)" onclick="switchView('products');setTimeout(()=>window._loadProduct('${esc(taskProduct.id)}'),300)">${esc(taskProduct.marker)} ${esc(taskProduct.title)}</span>` : ''}
            ${t.manifest_id ? `<span style="opacity:0.4"> → </span><span style="cursor:pointer;color:var(--accent)" onclick="switchView('manifests');setTimeout(()=>window._loadManifest('${esc(t.manifest_id)}'),300)">${esc(taskManifest ? taskManifest.marker + ' ' + taskManifest.title : t.manifest_id.substring(0,12))}</span>` : ''}
            <span style="opacity:0.4"> → </span>
            <span style="color:var(--text-primary)">${esc(t.marker)} ${esc(t.title)}</span>
          </div>
          <!-- 1. HEADER BAR -->
          <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
            <span class="session-uuid" style="font-size:14px">${esc(t.marker)}</span>
            <span style="flex:1;font-size:15px;font-weight:600;color:var(--text-primary)">${esc(t.title)}</span>
            <span style="color:${statusColor};font-weight:700;font-size:13px;text-transform:uppercase;padding:3px 10px;border:2px solid ${statusColor};border-radius:4px">${esc(t.status)}</span>
          </div>

          <!-- METADATA — runs, turns, cost, agent, manifest, branch -->
          <div style="display:flex;gap:12px;font-size:12px;color:var(--text-muted);margin-bottom:16px;align-items:center;flex-wrap:wrap;padding:8px 12px;background:var(--bg-secondary);border:1px solid var(--border);border-radius:6px;font-family:var(--font-mono)">
            <span>Runs: <strong style="color:var(--text-primary)">${t.run_count}</strong></span>
            <span>Turns: <strong style="color:var(--text-primary)">${t.total_turns || 0}</strong></span>
            <span>Cost: <strong style="color:var(--green)">$${(t.total_cost || 0).toFixed(2)}</strong></span>
            <span style="opacity:0.3">|</span>
            <span>Agent: <strong style="color:var(--text-primary)">${esc(t.agent)}</strong></span>
            <span>Branch: <strong style="color:var(--text-primary)">openloom/${esc(t.marker)}</strong></span>
            ${t.manifest_id ? `<span>Manifest: <span class="manifest-nav" style="cursor:pointer;color:var(--accent);text-decoration:underline;font-weight:600" data-mid="${esc(t.manifest_id)}">${esc(t.manifest_id.substring(0,12))} &#x2192;</span></span>` : '<span>standalone</span>'}
            <span style="opacity:0.3">|</span>
            <span style="display:flex;align-items:center;gap:6px">Max turns: <input type="range" id="task-max-turns" value="${t.max_turns || 100}" min="10" max="500" step="10" style="width:100px;accent-color:var(--accent);cursor:pointer" oninput="document.getElementById('task-max-turns-val').textContent=this.value" onchange="window._updateMaxTurns('${esc(t.id)}',this.value)" /><strong id="task-max-turns-val" style="color:var(--text-primary);min-width:28px">${t.max_turns || 100}</strong></span>
            ${t.last_run_at ? `<span style="opacity:0.3">|</span><span>Last: ${new Date(t.last_run_at).toLocaleString()}</span>` : ''}
            <span>Created: ${new Date(t.created_at).toLocaleString()}</span>
          </div>

          <!-- 2. CONTROL BAR — ALWAYS VISIBLE -->
          <div style="margin-bottom:16px;padding:12px;border:1px solid var(--border);border-radius:8px;background:var(--bg-secondary)">
            <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:${isRunningOrPaused ? '0' : '12'}px">
              ${t.status === 'pending' || t.status === 'waiting' ? `
                <button class="btn-search" onclick="window._taskStart('${esc(t.id)}')">&#9654; Start Now</button>
                <button class="btn-search" style="background:var(--bg-input)" onclick="window._rescheduleTask('${esc(t.id)}')">&#128260; Reschedule</button>
                <button class="btn-dismiss" onclick="window._taskArchive('${esc(t.id)}')">Archive</button>
              ` : ''}
              ${t.status === 'scheduled' ? `
                <button class="btn-search" onclick="window._taskStart('${esc(t.id)}')">&#9654; Start Now</button>
                <button class="btn-dismiss" onclick="window._taskAction('${esc(t.id)}','cancel')">&#10005; Cancel</button>
                <button class="btn-search" style="background:var(--bg-input)" onclick="window._rescheduleTask('${esc(t.id)}')">&#128260; Reschedule</button>
                <button class="btn-dismiss" onclick="window._taskArchive('${esc(t.id)}')">Archive</button>
              ` : ''}
              ${t.status === 'running' ? `
                <button class="btn-search" style="background:var(--yellow);color:var(--bg-primary)" onclick="window._pauseTask('${esc(t.id)}')">&#9208; Pause</button>
                <button class="btn-confirm" onclick="window._killTask('${esc(t.id)}')">&#9209; Stop</button>
              ` : ''}
              ${t.status === 'paused' ? `
                <button class="btn-search" onclick="window._resumeTask('${esc(t.id)}')">&#9654; Resume</button>
                <button class="btn-search" style="background:var(--accent)" onclick="window._editInstructions('${esc(t.id)}')">&#9998; Edit Instructions</button>
                <button class="btn-confirm" onclick="window._killTask('${esc(t.id)}')">&#9209; Stop</button>
              ` : ''}
              ${t.status === 'completed' || t.status === 'failed' || t.status === 'cancelled' ? `
                <button class="btn-search" onclick="window._taskStart('${esc(t.id)}')">&#128260; Restart</button>
                <button class="btn-search" style="background:var(--bg-input)" onclick="window._rescheduleTask('${esc(t.id)}')">&#128260; Reschedule</button>
                <button class="btn-dismiss" onclick="window._taskArchive('${esc(t.id)}')">Archive</button>
              ` : ''}
            </div>

            ${!isRunningOrPaused ? `
            <!-- 3. SCHEDULE SECTION — ALWAYS VISIBLE for non-running tasks -->
            <div style="border-top:1px solid var(--border);padding-top:12px">
              <div style="display:flex;align-items:center;gap:12px;margin-bottom:10px">
                <span style="font-size:12px;font-weight:600;color:var(--text-primary)">Schedule</span>
                <span style="font-size:20px">${isOneShot ? '&#9889;' : '&#128260;'}</span>
                <span style="font-size:12px;font-weight:700;color:${isOneShot ? 'var(--accent)' : 'var(--green)'}">${isOneShot ? 'ONE-SHOT' : 'RECURRING (every ' + esc(t.schedule) + ')'}</span>
              </div>

              <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-bottom:10px">
                <label style="font-size:11px;color:var(--text-muted);display:flex;align-items:center;gap:4px">
                  <input type="radio" name="task-sched-mode" value="oneshot" ${isOneShot ? 'checked' : ''} /> One-shot
                </label>
                <label style="font-size:11px;color:var(--text-muted);display:flex;align-items:center;gap:4px">
                  <input type="radio" name="task-sched-mode" value="recurring" ${!isOneShot ? 'checked' : ''} /> Recurring
                </label>
              </div>

              <!-- One-shot options -->
              <div id="task-sched-oneshot" style="display:${isOneShot ? 'flex' : 'none'};gap:6px;align-items:center;flex-wrap:wrap;margin-bottom:8px">
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${scheduleSelectVal==='once'?'border:2px solid var(--accent)':''}" onclick="window._quickSchedule('${esc(t.id)}','once')">Now</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;background:var(--bg-input);${scheduleSelectVal==='in:5m'?'border:2px solid var(--accent)':''}" onclick="window._quickSchedule('${esc(t.id)}','in:5m')">In 5m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;background:var(--bg-input);${scheduleSelectVal==='in:15m'?'border:2px solid var(--accent)':''}" onclick="window._quickSchedule('${esc(t.id)}','in:15m')">In 15m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;background:var(--bg-input);${scheduleSelectVal==='in:30m'?'border:2px solid var(--accent)':''}" onclick="window._quickSchedule('${esc(t.id)}','in:30m')">In 30m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;background:var(--bg-input);${scheduleSelectVal==='in:1h'?'border:2px solid var(--accent)':''}" onclick="window._quickSchedule('${esc(t.id)}','in:1h')">In 1h</button>
                <span style="font-size:11px;color:var(--text-muted)">At:</span>
                <input type="datetime-local" id="task-sched-at-picker" class="conv-search" style="font-size:11px;padding:4px" value="${t.next_run_at ? new Date(t.next_run_at).toISOString().slice(0,16) : ''}" />
                <button class="btn-search" style="font-size:11px;padding:4px 10px" onclick="window._scheduleAt('${esc(t.id)}')">Set</button>
              </div>

              <!-- Recurring options -->
              <div id="task-sched-recurring" style="display:${!isOneShot ? 'flex' : 'none'};gap:6px;align-items:center;flex-wrap:wrap;margin-bottom:8px">
                <span style="font-size:11px;color:var(--text-muted)">Every:</span>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='5m'?'border:2px solid var(--green)':''}" onclick="window._quickSchedule('${esc(t.id)}','5m')">5m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='15m'?'border:2px solid var(--green)':''}" onclick="window._quickSchedule('${esc(t.id)}','15m')">15m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='30m'?'border:2px solid var(--green)':''}" onclick="window._quickSchedule('${esc(t.id)}','30m')">30m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='1h'?'border:2px solid var(--green)':''}" onclick="window._quickSchedule('${esc(t.id)}','1h')">1h</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='6h'?'border:2px solid var(--green)':''}" onclick="window._quickSchedule('${esc(t.id)}','6h')">6h</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='24h'?'border:2px solid var(--green)':''}" onclick="window._quickSchedule('${esc(t.id)}','24h')">24h</button>
              </div>

              ${t.next_run_at ? `<div style="font-size:12px;color:var(--text-muted)">Next run: <strong style="color:var(--text-primary)">${new Date(t.next_run_at).toLocaleString()}</strong></div>` : ''}
            </div>
            ` : ''}
          </div>

          <!-- 3b. DEPENDS ON -->
          <div id="task-dependency-section" style="margin-bottom:16px;padding:12px;border:1px solid var(--border);border-radius:8px;background:var(--bg-secondary)">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span style="font-size:12px;font-weight:600;color:var(--text-primary)">Depends On</span>
              ${t.depends_on ? `<span class="badge scope" style="font-size:10px">CHAINED</span>` : `<span style="font-size:11px;color:var(--text-muted)">No dependency</span>`}
            </div>
            <div id="task-dep-current">
              ${t.depends_on ? `<div id="task-dep-info" style="display:flex;align-items:center;gap:8px;padding:8px 10px;border:1px solid var(--border);border-radius:6px;background:var(--bg-primary);margin-bottom:8px">
                <span style="font-size:11px;color:var(--text-muted)">Loading...</span>
              </div>` : ''}
            </div>
            <div style="display:flex;gap:6px;align-items:center;flex-wrap:wrap">
              <input type="text" id="task-dep-search" class="conv-search" placeholder="Search tasks to set dependency..." style="font-size:12px;flex:1;min-width:180px;padding:6px 10px" />
              <select id="task-dep-select" class="conv-filter" style="font-size:12px;padding:6px 8px;max-width:320px;display:none">
              </select>
              <button id="task-dep-set-btn" class="btn-search" style="font-size:11px;padding:4px 12px;display:none" onclick="window._setDependency('${esc(t.id)}')">Set</button>
              ${t.depends_on ? `<button class="btn-dismiss" style="font-size:11px;padding:4px 12px" onclick="window._removeDependency('${esc(t.id)}')">Remove</button>` : ''}
            </div>
          </div>

          <!-- 4. INSTRUCTIONS (editable) -->
          <div id="task-instructions-section" style="margin-bottom:12px">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
              <span style="font-size:12px;font-weight:600;color:var(--text-primary)">Instructions</span>
              <button id="task-instructions-edit-btn" class="btn-search" style="font-size:11px;padding:2px 10px" onclick="window._editInstructions('${esc(t.id)}')">${t.description ? 'Edit' : 'Add Instructions'}</button>
            </div>
            <div id="task-instructions-display">
              ${t.description
                ? `<div style="font-size:13px;color:var(--text-secondary);padding:10px 12px;border:1px solid var(--border);border-radius:6px;background:var(--bg-secondary);white-space:pre-wrap;word-break:break-word;font-family:var(--font-mono);line-height:1.5;max-height:400px;overflow-y:auto">${esc(t.description)}</div>`
                : `<div style="font-size:12px;color:var(--text-muted);font-style:italic">No instructions</div>`}
            </div>
            <div id="task-instructions-editor" style="display:none">
              <textarea id="task-instructions-textarea" style="width:100%;min-height:300px;padding:12px;font-size:13px;line-height:1.5;font-family:var(--font-mono);resize:both;border:1px solid var(--accent);border-radius:6px;background:var(--bg-secondary);color:var(--text-primary);box-sizing:border-box">${esc(t.description || '')}</textarea>
              <div style="display:flex;gap:8px;margin-top:8px">
                <button class="btn-search" style="padding:4px 14px;font-size:12px" onclick="window._saveInstructions('${esc(t.id)}')">Save</button>
                <button class="btn-dismiss" style="padding:4px 14px;font-size:12px" onclick="window._cancelEditInstructions()">Cancel</button>
              </div>
            </div>
          </div>

          <!-- META INFO (now in header above) -->

          <!-- 5. LIVE OUTPUT (if running/paused) -->
          ${isRunningOrPaused ? `
            <div style="margin-bottom:12px;border-left:3px solid ${t.status === 'paused' ? 'var(--yellow)' : 'var(--green)'};padding-left:10px">
              <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
                <span class="status-dot ${t.status === 'paused' ? 'yellow' : 'green'} ${t.status === 'paused' ? '' : 'status-pulse'}"></span>
                <span style="font-size:12px;color:${t.status === 'paused' ? 'var(--yellow)' : 'var(--green)'};font-weight:500">${t.status === 'paused' ? 'PAUSED' : 'LIVE OUTPUT'}</span>
              </div>
              <div class="action-response" id="task-live-output" style="max-height:300px;overflow-y:auto;font-size:11px">Loading...</div>
            </div>
          ` : ''}

          <!-- VIOLATIONS / DEVIATIONS -->
          ${amnesiaHtml}
          ${delusionsHtml}

          <!-- ACTIONS -->
          ${actionsHtml}

          <!-- 6. RUN HISTORY -->
          ${runsHtml}

          <!-- LAST OUTPUT -->
          ${t.last_output && t.status !== 'running' ? `
            <div style="font-size:12px;color:var(--text-muted);margin-bottom:6px;font-weight:500">Output</div>
            <div id="task-parsed-output" style="max-height:400px;overflow-y:auto"></div>
            <div style="margin-top:6px">
              <span style="font-size:11px;color:var(--text-muted);cursor:pointer;text-decoration:underline" onclick="document.getElementById('task-raw-output').style.display=document.getElementById('task-raw-output').style.display==='none'?'':'none'">Toggle raw JSON</span>
            </div>
            <div class="action-response" id="task-raw-output" style="max-height:200px;overflow-y:auto;display:none;font-size:10px">${esc(t.last_output.length > 50000 ? t.last_output.substring(0, 50000) + '\n... (truncated for display, full output in DB)' : t.last_output)}</div>
          ` : ''}

          <!-- FOOTER -->
          <div style="margin-top:12px;font-family:var(--font-mono);font-size:10px;color:var(--text-muted)">
            Node: ${esc(t.source_node)} | ID: ${esc(t.id)}
          </div>
        </div>
      `;

      // Manifest cross-link — fetch title + wire click
      bodyEl.querySelectorAll('.manifest-nav').forEach(el => {
        const mid = el.dataset.mid;
        // Fetch manifest title to show instead of just the ID
        fetchJSON('/api/manifests/' + mid).then(m => {
          if (m && m.title) {
            const titleSpan = el.querySelector('.manifest-nav-title');
            if (titleSpan) titleSpan.textContent = m.title;
          }
        }).catch(() => {});
        el.addEventListener('click', () => {
          switchView('manifests');
          setTimeout(() => window._loadManifest(mid), 300);
        });
      });

      // Action cross-links
      bodyEl.querySelectorAll('.action-link').forEach(el => {
        el.addEventListener('click', () => {
          switchView('actions');
          setTimeout(() => loadActionDetail(el.dataset.aid), 300);
        });
      });

      // Run history click — show output for that run
      bodyEl.querySelectorAll('.task-run-item').forEach(el => {
        el.addEventListener('click', () => {
          const runId = el.dataset.runId;
          const taskId = el.dataset.taskId;
          loadRunOutput(taskId, runId, el);
        });
      });

      // Schedule mode radio toggle — show/hide one-shot vs recurring options
      bodyEl.querySelectorAll('input[name="task-sched-mode"]').forEach(radio => {
        radio.addEventListener('change', () => {
          const oneshotEl = bodyEl.querySelector('#task-sched-oneshot');
          const recurringEl = bodyEl.querySelector('#task-sched-recurring');
          if (oneshotEl && recurringEl) {
            if (radio.value === 'oneshot') {
              oneshotEl.style.display = 'flex';
              recurringEl.style.display = 'none';
            } else {
              oneshotEl.style.display = 'none';
              recurringEl.style.display = 'flex';
            }
          }
        });
      });

      // Dependency picker — load current dep info + wire search/select
      (async () => {
        const depSearch = bodyEl.querySelector('#task-dep-search');
        const depSelect = bodyEl.querySelector('#task-dep-select');
        const depSetBtn = bodyEl.querySelector('#task-dep-set-btn');
        const depInfo = bodyEl.querySelector('#task-dep-info');

        // Show current dependency info
        if (t.depends_on && depInfo) {
          try {
            const dep = await fetchJSON('/api/tasks/' + t.depends_on);
            if (dep) {
              const depStatusColor = dep.status === 'running' ? 'var(--green)' : dep.status === 'completed' ? 'var(--green)' : dep.status === 'failed' ? 'var(--red)' : 'var(--yellow)';
              depInfo.innerHTML = `
                <span class="session-uuid" style="font-size:11px">${esc(dep.marker)}</span>
                <span style="font-size:12px;font-weight:600;color:var(--text-primary);cursor:pointer;text-decoration:underline" onclick="loadTaskDetail('${esc(dep.id)}')">${esc(dep.title)}</span>
                <span style="color:${depStatusColor};font-size:10px;font-weight:700;text-transform:uppercase;padding:2px 6px;border:1px solid ${depStatusColor};border-radius:3px">${esc(dep.status)}</span>
              `;
            } else {
              depInfo.innerHTML = `<span style="font-size:11px;color:var(--red)">Dependency task not found (${esc(t.depends_on.substring(0,12))})</span>`;
            }
          } catch(e) {
            depInfo.innerHTML = `<span style="font-size:11px;color:var(--red)">Failed to load dependency</span>`;
          }
        }

        // Load all tasks for picker
        if (depSearch) {
          let allTasks = [];
          try {
            const taskList = await fetchJSON('/api/tasks?status=');
            allTasks = (taskList || []).filter(tk => tk.id !== t.id);
          } catch(e) {}

          const populateOptions = (filter) => {
            depSelect.innerHTML = '';
            const q = (filter || '').toLowerCase();
            const matches = allTasks.filter(tk => !q || tk.title.toLowerCase().includes(q) || tk.marker.toLowerCase().includes(q) || tk.id.toLowerCase().includes(q));
            matches.slice(0, 30).forEach(tk => {
              const opt = document.createElement('option');
              opt.value = tk.id;
              opt.textContent = '[' + tk.marker + '] ' + tk.title + ' (' + tk.status + ')';
              depSelect.appendChild(opt);
            });
            const hasMatches = matches.length > 0 && q.length > 0;
            depSelect.style.display = hasMatches ? '' : 'none';
            depSetBtn.style.display = hasMatches ? '' : 'none';
          };

          depSearch.addEventListener('input', () => populateOptions(depSearch.value));
        }
      })();

      // Parse output into readable format
      if (t.last_output && t.status !== 'running' && t.status !== 'paused') {
        const parsedEl = document.getElementById('task-parsed-output');
        if (parsedEl) {
          parsedEl.innerHTML = parseTaskOutput(t.last_output);
        }
      }

      // Live output polling for running/paused tasks
      if (t.status === 'running' || t.status === 'paused') {
        const pollOutput = async () => {
          try {
            const data = await fetchJSON('/api/tasks/' + t.id + '/output');
            const liveEl = document.getElementById('task-live-output');
            if (!liveEl) return; // detail changed
            if (data && data.lines && data.lines.length) {
              liveEl.innerHTML = parseTaskOutput(data.lines.join('\n'));
              liveEl.scrollTop = liveEl.scrollHeight;
            } else {
              liveEl.innerHTML = '<span style="color:var(--text-muted)">Waiting for output...</span>';
            }
            if (data && data.running) {
              setTimeout(pollOutput, 2000);
            } else {
              // Task finished — reload full detail
              loadTaskDetail(t.id);
              loadTasks();
            }
          } catch (e) {}
        };
        pollOutput();
      }
    } catch (e) {
      console.error('Load task detail failed:', e);
    }
  }

  window._setDependency = async function(taskId) {
    const sel = document.getElementById('task-dep-select');
    if (!sel || !sel.value) return;
    await fetch('/api/tasks/' + taskId + '/dependency', {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({depends_on: sel.value})
    });
    loadTaskDetail(taskId);
    loadTasks();
  };

  window._removeDependency = async function(taskId) {
    await fetch('/api/tasks/' + taskId + '/dependency', {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({depends_on: ''})
    });
    loadTaskDetail(taskId);
    loadTasks();
  };

  window._taskAction = async function(id, action) {
    await fetch('/api/tasks/' + id + '/' + action, {method: 'POST'});
    loadTaskDetail(id);
    loadTasks();
  };

  // Re-run now (uses existing schedule)
  window._updateMaxTurns = async function(id, val) {
    const maxTurns = parseInt(val);
    if (!maxTurns || maxTurns < 1) return;
    await fetch('/api/tasks/' + id, {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({max_turns: maxTurns})
    });
  };

  window._taskStart = async function(id) {
    await fetch('/api/tasks/' + id + '/start', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({schedule: 'once'})
    });
    loadTaskDetail(id);
    loadTasks();
  };

  // Quick schedule — one click reschedule to a preset
  window._quickSchedule = async function(id, schedule) {
    await fetch('/api/tasks/' + id + '/reschedule', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({schedule})
    });
    loadTaskDetail(id);
    loadTasks();
  };

  // Schedule at a specific datetime from the picker
  window._scheduleAt = async function(id) {
    const dt = document.getElementById('task-sched-at-picker');
    if (!dt || !dt.value) { alert('Pick a date/time first'); return; }
    const schedule = 'at:' + new Date(dt.value).toISOString();
    await fetch('/api/tasks/' + id + '/reschedule', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({schedule})
    });
    loadTaskDetail(id);
    loadTasks();
  };

  // Reschedule — called from the Reschedule button in control bar
  window._rescheduleTask = async function(id) {
    // Just scroll to the schedule section — it's always visible
    const schedSection = document.querySelector('#task-sched-oneshot, #task-sched-recurring');
    if (schedSection) {
      schedSection.scrollIntoView({behavior: 'smooth', block: 'center'});
      schedSection.style.outline = '2px solid var(--accent)';
      setTimeout(() => { schedSection.style.outline = ''; }, 1500);
    }
  };

  // Load and display output for a specific run
  async function loadRunOutput(taskId, runId, clickedEl) {
    // Toggle — if already showing, hide
    const existing = clickedEl.querySelector('.run-output-panel');
    if (existing) {
      existing.remove();
      return;
    }
    // Remove other open panels
    document.querySelectorAll('.run-output-panel').forEach(p => p.remove());

    const panel = document.createElement('div');
    panel.className = 'run-output-panel';
    panel.style.cssText = 'margin-top:6px;padding:6px;border-top:1px solid var(--border)';
    panel.innerHTML = '<span style="font-size:10px;color:var(--text-muted)">Loading...</span>';
    clickedEl.appendChild(panel);

    try {
      const run = await fetchJSON('/api/tasks/' + taskId + '/runs/' + runId);
      if (run && run.output) {
        panel.innerHTML = `<pre class="action-response" style="max-height:200px;overflow-y:auto;font-size:10px;margin:0">${esc(run.output)}</pre>`;
      } else {
        panel.innerHTML = '<span style="font-size:10px;color:var(--text-muted)">No output</span>';
      }
    } catch(e) {
      panel.innerHTML = '<span style="font-size:10px;color:var(--red)">Failed to load</span>';
    }
  }

  // Format a duration in ms to human-readable
  function formatDuration(ms) {
    if (ms < 1000) return ms + 'ms';
    const s = Math.floor(ms / 1000);
    if (s < 60) return s + 's';
    const m = Math.floor(s / 60);
    const rs = s % 60;
    if (m < 60) return m + 'm ' + rs + 's';
    const h = Math.floor(m / 60);
    const rm = m % 60;
    return h + 'h ' + rm + 'm';
  }

  async function showTaskCreateForm() {
    const titleEl = document.getElementById('task-detail-title');
    const bodyEl = document.getElementById('task-detail');
    titleEl.textContent = 'New Task';

    // Fetch manifests for the dropdown
    let manifests = [];
    try {
      manifests = await fetchJSON('/api/manifests');
    } catch(e) {}

    const manifestOptions = (manifests || []).map(m =>
      `<option value="${esc(m.id)}">[${esc(m.marker)}] ${esc(m.title)}</option>`
    ).join('');

    bodyEl.innerHTML = `
      <div>
        <div style="margin-bottom:16px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Manifest <span style="opacity:0.5">(optional — leave blank for standalone task)</span></label>
          <input type="text" id="tc-manifest-search" class="conv-search" placeholder="Search manifests..." style="font-size:13px;margin-bottom:6px" />
          <select id="tc-manifest-id" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
            <option value="">No manifest (standalone)</option>
            ${manifestOptions}
          </select>
        </div>
        <div style="margin-bottom:16px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Task Title</label>
          <input type="text" id="tc-title" class="conv-search" placeholder="What should the agent do?" style="font-size:13px" />
        </div>
        <div style="margin-bottom:16px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Instructions</label>
          <textarea id="tc-description" class="conv-search" placeholder="What should the agent do? These instructions will be executed exactly as written..." style="font-size:14px;min-height:300px;width:100%;resize:both;font-family:monospace;padding:12px;line-height:1.5;box-sizing:border-box"></textarea>
          <div id="tc-instructions-error" style="font-size:12px;color:var(--red);margin-top:4px;display:none">Instructions are required — the agent needs to know what to do</div>
        </div>
        <div style="display:flex;gap:12px;margin-bottom:16px">
          <div style="flex:1">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Schedule</label>
            <select id="tc-schedule-type" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
              <option value="at">Run at specific time</option>
              <option value="once">Run once now</option>
              <option value="5m">Every 5 minutes</option>
              <option value="15m">Every 15 minutes</option>
              <option value="30m">Every 30 minutes</option>
              <option value="1h">Every 1 hour</option>
              <option value="6h">Every 6 hours</option>
              <option value="24h">Every 24 hours</option>
            </select>
          </div>
          <div style="flex:1">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Agent</label>
            <select id="tc-agent" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
              <option value="claude-code">Claude Code</option>
              <option value="cursor">Cursor</option>
              <option value="copilot">Copilot</option>
              <option value="custom">Custom</option>
            </select>
          </div>
          <div style="flex:1">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Max Turns</label>
            <select id="tc-max-turns" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
              <option value="10">10</option>
              <option value="25">25</option>
              <option value="50">50</option>
              <option value="100" selected>100 (default)</option>
              <option value="200">200</option>
            </select>
          </div>
        </div>
        <div id="tc-datetime-row" style="margin-bottom:16px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Run At</label>
          <input type="datetime-local" id="tc-run-at" class="conv-search" style="font-size:13px" />
        </div>
        <div style="display:flex;gap:8px">
          <button id="tc-submit" class="btn-search" style="padding:8px 20px">Create Task</button>
          <button id="tc-cancel" class="btn-dismiss" style="padding:8px 16px">Cancel</button>
          <span id="tc-status" style="font-size:13px;color:var(--green);align-self:center"></span>
        </div>
      </div>
    `;

    // Manifest search filter
    const searchInput = bodyEl.querySelector('#tc-manifest-search');
    const selectEl = bodyEl.querySelector('#tc-manifest-id');
    const allOptions = Array.from(selectEl.options);
    searchInput.addEventListener('input', () => {
      const q = searchInput.value.toLowerCase();
      selectEl.innerHTML = '';
      allOptions.forEach(opt => {
        if (!q || opt.textContent.toLowerCase().includes(q) || opt.value.toLowerCase().includes(q)) {
          selectEl.appendChild(opt.cloneNode(true));
        }
      });
    });

    // Show/hide datetime
    const schedType = bodyEl.querySelector('#tc-schedule-type');
    const dtRow = bodyEl.querySelector('#tc-datetime-row');
    schedType.onchange = () => {
      dtRow.style.display = schedType.value === 'at' ? '' : 'none';
    };

    // Cancel
    bodyEl.querySelector('#tc-cancel').onclick = () => {
      titleEl.textContent = 'Select a task';
      bodyEl.innerHTML = '<div class="empty-state">Click a task to view details, or click <strong>+ New Task</strong> to create one</div>';
    };

    // Submit
    bodyEl.querySelector('#tc-submit').onclick = async () => {
      const manifestId = selectEl.value;
      const title = bodyEl.querySelector('#tc-title').value.trim();
      const desc = bodyEl.querySelector('#tc-description').value.trim();
      const sched = schedType.value;
      const agent = bodyEl.querySelector('#tc-agent').value;
      const maxTurns = parseInt(bodyEl.querySelector('#tc-max-turns').value) || 100;
      const instrErr = bodyEl.querySelector('#tc-instructions-error');
      if (!desc) { instrErr.style.display = 'block'; return; } else { instrErr.style.display = 'none'; }
      if (!manifestId && !title) { bodyEl.querySelector('#tc-status').textContent = 'Title is required for standalone tasks'; return; }

      let schedule = sched;
      if (sched === 'at') {
        const dt = bodyEl.querySelector('#tc-run-at').value;
        if (!dt) { bodyEl.querySelector('#tc-status').textContent = 'Set a date/time'; return; }
        schedule = 'at:' + new Date(dt).toISOString();
      }

      await fetch('/api/tasks', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({manifest_id: manifestId, title, description: desc, schedule, agent, max_turns: maxTurns})
      });
      bodyEl.querySelector('#tc-status').textContent = 'Created!';
      loadTasks();
      setTimeout(() => {
        titleEl.textContent = 'Select a task';
        bodyEl.innerHTML = '<div class="empty-state">Task created. Click it in the tree to view details.</div>';
      }, 1000);
    };
  }

  // Edit instructions — toggle editor view
  window._editInstructions = function(id) {
    const display = document.getElementById('task-instructions-display');
    const editor = document.getElementById('task-instructions-editor');
    const btn = document.getElementById('task-instructions-edit-btn');
    if (!display || !editor) return;
    display.style.display = 'none';
    editor.style.display = '';
    if (btn) btn.style.display = 'none';
    // Scroll to instructions section
    const section = document.getElementById('task-instructions-section');
    if (section) section.scrollIntoView({behavior: 'smooth', block: 'center'});
    // Focus textarea
    const ta = document.getElementById('task-instructions-textarea');
    if (ta) { ta.focus(); ta.setSelectionRange(ta.value.length, ta.value.length); }
  };

  // Save instructions via PATCH
  window._saveInstructions = async function(id) {
    const ta = document.getElementById('task-instructions-textarea');
    if (!ta) return;
    await fetch('/api/tasks/' + id, {
      method: 'PATCH',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({description: ta.value})
    });
    loadTaskDetail(id);
  };

  // Cancel edit — revert to display view
  window._cancelEditInstructions = function() {
    const display = document.getElementById('task-instructions-display');
    const editor = document.getElementById('task-instructions-editor');
    const btn = document.getElementById('task-instructions-edit-btn');
    if (display) display.style.display = '';
    if (editor) editor.style.display = 'none';
    if (btn) btn.style.display = '';
  };

  window._taskArchive = async function(id) {
    await fetch('/api/tasks/' + id + '/cancel', {method: 'POST'});
    loadTasks();
    loadTaskDetail(id);
  };

  // --- Cost History v3 ---
  let _costPeriod = 'day';
  let _costAgent = '';
  let _costThreshold = 0; // 0 = use server default
  let _lastCostData = null; // for CSV export
  let _lastDrillData = null; // for CSV export in drill-down

  // Agent color palette for drill-down grouping
  const _agentColors = ['var(--accent)', 'var(--green)', 'var(--yellow)', '#e879f9', '#fb923c', '#38bdf8', '#f87171', '#a78bfa'];
  function agentColor(agent, agents) {
    const idx = agents.indexOf(agent);
    return _agentColors[idx >= 0 ? idx % _agentColors.length : 0];
  }

  function getSelectedAgent() {
    const el = document.getElementById('cost-agent-select');
    return el ? el.value : '';
  }

  function getEffectiveBudget(serverBudget) {
    return _costThreshold > 0 ? _costThreshold : (serverBudget || 0);
  }

  // Load agent dropdown options
  async function loadCostAgents() {
    try {
      const agents = await fetchJSON('/api/tasks/cost-agents');
      const sel = document.getElementById('cost-agent-select');
      if (!sel || !agents) return;
      const current = sel.value;
      sel.innerHTML = '<option value="">All Agents</option>';
      for (const a of agents) {
        sel.innerHTML += `<option value="${esc(a)}"${a === current ? ' selected' : ''}>${esc(a)}</option>`;
      }
    } catch(e) {}
  }

  // Load trend summary cards
  async function loadCostTrend() {
    try {
      const agent = getSelectedAgent();
      const agentParam = agent ? '&agent=' + encodeURIComponent(agent) : '';
      const trend = await fetchJSON('/api/tasks/cost-trend?' + agentParam);
      if (!trend) return;
      const budget = getEffectiveBudget(100);
      const todayEl = document.getElementById('trend-today');
      const weekEl = document.getElementById('trend-week');
      const monthEl = document.getElementById('trend-month');
      const avgEl = document.getElementById('trend-avg');
      if (todayEl) {
        todayEl.textContent = '$' + trend.today.toFixed(2);
        todayEl.style.color = (budget > 0 && trend.today >= budget) ? 'var(--red)' : (budget > 0 && trend.today >= budget * 0.8) ? 'var(--yellow)' : 'var(--green)';
      }
      if (weekEl) weekEl.textContent = '$' + trend.this_week.toFixed(2);
      if (monthEl) monthEl.textContent = '$' + trend.this_month.toFixed(2);
      if (avgEl) avgEl.textContent = '$' + trend.avg_30d.toFixed(2);
    } catch(e) {}
  }

  async function loadCostHistory() {
    const tableEl = document.getElementById('cost-history-table');
    const chartEl = document.getElementById('cost-chart');
    const banner = document.getElementById('cost-drilldown-banner');
    if (!tableEl) return;

    loadCostAgents();
    loadCostTrend();

    const sel = document.getElementById('cost-days-select');
    const days = sel ? parseInt(sel.value) || 30 : 30;
    const agent = getSelectedAgent();
    const agentParam = agent ? '&agent=' + encodeURIComponent(agent) : '';

    try {
      const data = await fetchJSON('/api/tasks/cost-history?days=' + days + '&period=' + _costPeriod + agentParam);
      if (!data || !data.length) {
        tableEl.innerHTML = '<div class="empty-state">No cost data yet</div>';
        if (chartEl) chartEl.innerHTML = '<div class="empty-state">No data</div>';
        if (banner) banner.style.display = 'none';
        _lastCostData = null;
        return;
      }
      if (banner) banner.style.display = 'none';
      _lastCostData = data;
      _lastDrillData = null;

      const serverBudget = data[0].budget || 0;
      const budget = getEffectiveBudget(serverBudget);
      const periodLabel = _costPeriod === 'week' ? 'Week' : _costPeriod === 'month' ? 'Month' : 'Date';

      // Chart
      if (chartEl) {
        const reversed = [...data].reverse();
        const maxCost = Math.max(...reversed.map(d => d.cost), budget || 1, 1);
        const barWidth = Math.max(4, Math.floor((chartEl.clientWidth - 40) / reversed.length) - 2);
        let chartHtml = '<div style="display:flex;align-items:flex-end;gap:2px;height:100px;padding:8px 16px;position:relative">';
        if (budget > 0 && _costPeriod === 'day') {
          const budgetPx = Math.round((budget / maxCost) * 100);
          chartHtml += `<div style="position:absolute;left:16px;right:16px;bottom:${budgetPx + 8}px;border-top:2px dashed var(--red);opacity:0.5"></div>`;
          chartHtml += `<div style="position:absolute;right:20px;bottom:${budgetPx + 10}px;font-size:9px;color:var(--red);opacity:0.7;font-weight:600">$${budget.toFixed(0)} budget</div>`;
        }
        for (const d of reversed) {
          const h = maxCost > 0 ? Math.max(2, Math.round((d.cost / maxCost) * 100)) : 2;
          const overBudget = budget > 0 && d.cost >= budget;
          const color = overBudget ? 'var(--red)' : (budget > 0 && d.cost >= budget * 0.8) ? 'var(--yellow)' : 'var(--green)';
          chartHtml += `<div title="${d.period}: $${d.cost.toFixed(2)}" style="width:${barWidth}px;height:${h}px;background:${color};border-radius:2px 2px 0 0;flex-shrink:0;opacity:${d.cost === 0 ? '0.2' : '0.8'};cursor:${_costPeriod === 'day' ? 'pointer' : 'default'}" ${_costPeriod === 'day' ? `onclick="loadCostDrillDown('${d.period}')"` : ''}></div>`;
        }
        chartHtml += '</div>';
        chartEl.innerHTML = chartHtml;
      }

      // Table header
      const headerEl = document.getElementById('cost-table-header');
      if (headerEl) headerEl.textContent = _costPeriod === 'week' ? 'Weekly Breakdown' : _costPeriod === 'month' ? 'Monthly Breakdown' : 'Daily Breakdown';

      let html = `<table class="top-tasks-table" style="width:100%">
        <thead><tr>
          <th style="text-align:left;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">${periodLabel}</th>
          <th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">tasks</th>
          <th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">runs</th>
          <th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">turns</th>
          <th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">cost</th>
          <th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">status</th>
        </tr></thead><tbody>`;

      let totalCost = 0, totalTurns = 0, totalTasks = 0, totalRuns = 0;
      for (const d of data) {
        const overBudget = budget > 0 && d.cost >= budget && _costPeriod === 'day';
        const nearBudget = budget > 0 && d.cost >= budget * 0.8 && _costPeriod === 'day';
        const costColor = overBudget ? 'var(--red)' : nearBudget ? 'var(--yellow)' : 'var(--green)';
        const statusText = (_costPeriod === 'day' && budget > 0) ? (overBudget ? 'OVER' : 'OK') : '-';
        const statusColor = overBudget ? 'var(--red)' : 'var(--green)';
        const isToday = d.period === new Date().toISOString().split('T')[0];
        const rowBg = isToday ? 'background:rgba(0,217,126,0.04)' : '';
        const clickable = _costPeriod === 'day' && d.cost > 0;
        totalCost += d.cost;
        totalTurns += d.turns;
        totalTasks += d.tasks;
        totalRuns += (d.runs || 0);
        html += `<tr class="top-task-row" style="${rowBg};${clickable ? 'cursor:pointer' : ''}" ${clickable ? `onclick="loadCostDrillDown('${d.period}')"` : ''}>
          <td style="padding:6px 12px;font-family:var(--font-mono);font-size:12px;color:var(--accent)">${d.period}${isToday ? ' <span style="font-size:10px;color:var(--green)">(today)</span>' : ''}</td>
          <td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">${d.tasks}</td>
          <td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">${d.runs || d.tasks}</td>
          <td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">${d.turns}</td>
          <td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;color:${costColor}">$${d.cost.toFixed(2)}</td>
          <td style="padding:6px 12px;text-align:right;font-size:11px;font-weight:600;color:${statusColor}">${statusText}</td>
        </tr>`;
      }

      const avgCost = data.length > 0 ? totalCost / data.length : 0;
      const avgLabel = _costPeriod === 'week' ? '/week' : _costPeriod === 'month' ? '/month' : '/day';
      html += `<tr style="border-top:2px solid var(--border)">
        <td style="padding:8px 12px;font-size:12px;font-weight:600;color:var(--text-secondary)">Total / Avg</td>
        <td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600">${totalTasks}</td>
        <td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600">${totalRuns || totalTasks}</td>
        <td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600">${totalTurns}</td>
        <td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600;color:var(--green)">$${totalCost.toFixed(2)}</td>
        <td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:11px;color:var(--text-muted)">avg $${avgCost.toFixed(2)}${avgLabel}</td>
      </tr>`;
      html += '</tbody></table>';
      tableEl.innerHTML = html;

    } catch (e) {
      tableEl.innerHTML = '<div class="empty-state">Failed to load cost history</div>';
    }
  }

  // Drill-down: show individual tasks for a specific date, grouped by agent
  window.loadCostDrillDown = async function(date) {
    const tableEl = document.getElementById('cost-history-table');
    const chartEl = document.getElementById('cost-chart');
    const banner = document.getElementById('cost-drilldown-banner');
    const bannerLabel = document.getElementById('cost-drilldown-label');
    if (!tableEl) return;

    const agent = getSelectedAgent();
    const agentParam = agent ? '&agent=' + encodeURIComponent(agent) : '';

    try {
      const data = await fetchJSON('/api/tasks/cost-history?date=' + date + agentParam);
      if (!data || !data.entries || !data.entries.length) {
        tableEl.innerHTML = '<div class="empty-state">No tasks found for ' + date + '</div>';
        _lastDrillData = null;
        return;
      }
      _lastDrillData = { date, entries: data.entries };
      _lastCostData = null;

      // Show drilldown banner
      if (banner && bannerLabel) {
        banner.style.display = 'flex';
        bannerLabel.innerHTML = `Showing tasks for <strong style="color:var(--accent)">${date}</strong> (${data.entries.length} runs, $${data.entries.reduce((s,e) => s + e.cost_usd, 0).toFixed(2)} total)`;
      }

      // Hide chart in drilldown
      if (chartEl) chartEl.innerHTML = '';
      const headerEl = document.getElementById('cost-chart-header');
      if (headerEl) headerEl.textContent = 'Tasks on ' + date;

      // Group entries by agent
      const agentGroups = {};
      for (const e of data.entries) {
        const a = e.agent || 'unknown';
        if (!agentGroups[a]) agentGroups[a] = [];
        agentGroups[a].push(e);
      }
      const agentNames = Object.keys(agentGroups).sort();

      let html = '';
      for (const agentName of agentNames) {
        const entries = agentGroups[agentName];
        const groupCost = entries.reduce((s, e) => s + e.cost_usd, 0);
        const groupTurns = entries.reduce((s, e) => s + e.turns, 0);
        const color = agentColor(agentName, agentNames);

        // Agent group header
        html += `<div style="padding:8px 12px;background:var(--bg-secondary);border-bottom:1px solid var(--border);display:flex;align-items:center;gap:8px">
          <span style="width:8px;height:8px;border-radius:50%;background:${color};flex-shrink:0"></span>
          <strong style="font-size:12px;color:${color}">${esc(agentName)}</strong>
          <span style="font-size:11px;color:var(--text-muted);margin-left:auto">${entries.length} runs</span>
          <span style="font-size:11px;font-family:var(--font-mono);color:var(--text-secondary)">${groupTurns} turns</span>
          <span style="font-size:12px;font-family:var(--font-mono);font-weight:600;color:${color}">$${groupCost.toFixed(2)}</span>
        </div>`;

        html += `<table class="top-tasks-table" style="width:100%">
          <thead><tr>
            <th style="text-align:left;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">task</th>
            <th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">run</th>
            <th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">turns</th>
            <th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">actions</th>
            <th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">cost</th>
            <th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">duration</th>
            <th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">status</th>
          </tr></thead><tbody>`;

        for (const e of entries) {
          const statusColor = e.status === 'completed' ? 'var(--green)' : e.status === 'failed' ? 'var(--red)' : 'var(--yellow)';
          const dur = e.duration_sec > 0 ? formatDuration(e.duration_sec * 1000) : '-';
          const marker = e.task_marker || e.task_id.substring(0, 12);
          html += `<tr class="top-task-row" style="cursor:pointer" onclick="switchView('tasks');setTimeout(()=>loadTaskDetail('${esc(e.task_id)}'),300)">
            <td style="padding:5px 12px;font-size:12px">
              <span class="session-uuid" style="font-size:11px">${esc(marker)}</span>
              <span style="color:var(--text-secondary);margin-left:4px">${esc(e.task_title || 'Unknown')}</span>
              ${e.manifest_id ? `<span class="badge type" style="font-size:9px;margin-left:4px">${esc(e.manifest_id.substring(0,12))}</span>` : ''}
            </td>
            <td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">#${e.run_number}</td>
            <td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">${e.turns}</td>
            <td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">${e.actions}</td>
            <td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;color:var(--green)">$${e.cost_usd.toFixed(2)}</td>
            <td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;color:var(--text-muted)">${dur}</td>
            <td style="padding:5px 12px;text-align:right;font-size:11px;font-weight:600;color:${statusColor};text-transform:uppercase">${esc(e.status)}</td>
          </tr>`;
        }
        html += '</tbody></table>';
      }

      tableEl.innerHTML = html;

    } catch (e) {
      tableEl.innerHTML = '<div class="empty-state">Failed to load drill-down</div>';
    }
  };

  // CSV export
  function exportCostCSV() {
    let csv = '';
    if (_lastDrillData && _lastDrillData.entries) {
      csv = 'date,task_id,task_marker,task_title,agent,run_number,status,actions,cost_usd,turns,duration_sec,started_at,completed_at\n';
      for (const e of _lastDrillData.entries) {
        csv += [_lastDrillData.date, e.task_id, e.task_marker, '"' + (e.task_title || '').replace(/"/g, '""') + '"', e.agent || '', e.run_number, e.status, e.actions, e.cost_usd.toFixed(4), e.turns, e.duration_sec || 0, e.started_at, e.completed_at].join(',') + '\n';
      }
    } else if (_lastCostData) {
      csv = 'period,tasks,runs,turns,cost,budget\n';
      for (const d of _lastCostData) {
        csv += [d.period, d.tasks, d.runs || d.tasks, d.turns, d.cost.toFixed(4), d.budget || 0].join(',') + '\n';
      }
    } else {
      return;
    }
    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'openloom-cost-' + new Date().toISOString().split('T')[0] + '.csv';
    a.click();
    URL.revokeObjectURL(url);
  }

  // Cost history controls
  document.getElementById('cost-back-btn')?.addEventListener('click', () => switchView('overview'));
  document.getElementById('cost-days-select')?.addEventListener('change', () => loadCostHistory());
  document.getElementById('cost-drilldown-close')?.addEventListener('click', () => loadCostHistory());
  document.getElementById('cost-agent-select')?.addEventListener('change', () => { _costAgent = getSelectedAgent(); loadCostHistory(); });
  document.getElementById('cost-export-csv')?.addEventListener('click', exportCostCSV);

  // Threshold selector
  (function initThreshold() {
    const saved = localStorage.getItem('openloom-cost-threshold');
    if (saved) {
      _costThreshold = parseFloat(saved) || 0;
      const sel = document.getElementById('cost-threshold-select');
      if (sel) {
        const match = Array.from(sel.options).find(o => o.value === String(_costThreshold));
        if (match) sel.value = String(_costThreshold);
        else if (_costThreshold > 0) {
          sel.value = 'custom';
          const customEl = document.getElementById('cost-threshold-custom');
          if (customEl) { customEl.style.display = 'block'; customEl.value = _costThreshold; }
        }
      }
    }
  })();

  document.getElementById('cost-threshold-select')?.addEventListener('change', function() {
    const customEl = document.getElementById('cost-threshold-custom');
    if (this.value === 'custom') {
      if (customEl) customEl.style.display = 'block';
      return;
    }
    if (customEl) customEl.style.display = 'none';
    _costThreshold = parseFloat(this.value) || 0;
    localStorage.setItem('openloom-cost-threshold', String(_costThreshold));
    loadCostHistory();
  });

  document.getElementById('cost-threshold-custom')?.addEventListener('change', function() {
    const val = parseFloat(this.value) || 0;
    if (val > 0) {
      _costThreshold = val;
      localStorage.setItem('openloom-cost-threshold', String(_costThreshold));
      loadCostHistory();
    }
  });

  // Period tab clicks
  document.querySelectorAll('.cost-period-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      _costPeriod = btn.dataset.period;
      document.querySelectorAll('.cost-period-btn').forEach(b => {
        b.style.background = 'transparent';
        b.style.color = 'var(--text-secondary)';
        b.classList.remove('active');
      });
      btn.style.background = 'var(--accent)';
      btn.style.color = 'var(--bg)';
      btn.classList.add('active');
      loadCostHistory();
    });
  });

  // --- Delusions ---
  async function loadDelusions() {
    const el = document.getElementById('delusion-list');
    try {
      const peerGroups = await fetchJSON('/api/delusions/by-peer');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No delusions detected. Agents are following their manifests.</div>';
        return;
      }
      let html = '';
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];
        html += `<div class="tree-node peer-header clickable" data-del-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-del-peer-children="${pi}">`;
        for (const d of pg.delusions) {
          const sid = d.session_id ? d.session_id.substring(0, 12) : '';
          const simPercent = Math.round(d.score * 100);
          const statusClass = d.status === 'confirmed' ? 'confirmed' : d.status === 'dismissed' ? 'dismissed' : 'flagged';

          html += `<div class="amnesia-item ${statusClass}">
            <div class="amnesia-header">
              <span class="amnesia-score medium">${simPercent}% relevant</span>
              <span class="badge type">${esc(d.tool_name)}</span>
              ${d.action_id ? `<span class="badge scope action-link" style="cursor:pointer;font-size:10px" data-aid="${esc(d.action_id)}">view action</span>` : ''}
              <span class="amnesia-status-label">${esc(d.status)}</span>
              <span style="color:var(--text-muted);font-size:11px;margin-left:auto">${formatTime(d.created_at)}</span>
            </div>
            <div class="amnesia-session">
              <span style="color:var(--text-muted);font-size:12px">Session:</span>
              <span class="session-uuid">${esc(sid)}</span>
            </div>
            <div class="amnesia-rule">
              <span style="color:var(--yellow);font-weight:500">Manifest [${esc(d.manifest_marker)}]:</span> ${esc(d.manifest_title)}
            </div>
            <div style="font-size:12px;color:var(--text-secondary);margin-bottom:6px">${esc(d.reason)}</div>
            <div class="amnesia-action">
              <span style="color:var(--text-muted)">Action:</span> ${esc(d.tool_input)}
            </div>
            ${d.status === 'flagged' ? `<div class="amnesia-actions">
              <button class="btn-confirm" onclick="window._delusionAction(${d.id},'confirm')">Confirm Off-Spec</button>
              <button class="btn-dismiss" onclick="window._delusionAction(${d.id},'dismiss')">Dismiss</button>
            </div>` : ''}
          </div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer toggle
      el.querySelectorAll('[data-del-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.delPeer;
          const children = el.querySelector(`[data-del-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Action cross-links
      el.querySelectorAll('.action-link').forEach(link => {
        link.addEventListener('click', () => {
          switchView('actions');
          setTimeout(() => loadActionDetail(link.dataset.aid), 300);
        });
      });
    } catch (e) {
      console.error('Load delusions failed:', e);
    }
  }

  window._delusionAction = async function(id, action) {
    await fetch('/api/delusions/' + id + '/' + action, {method: 'POST'});
    loadDelusions();
  };

  async function updateDelusionCount() {
    try {
      const events = await fetchJSON('/api/delusions?status=flagged');
      const badge = document.getElementById('delusion-badge');
      if (events && events.length > 0) {
        badge.textContent = events.length;
        badge.style.display = '';
      } else {
        badge.style.display = 'none';
      }
    } catch (e) {}
  }

  // --- Amnesia ---
  async function loadAmnesia() {
    const el = document.getElementById('amnesia-list');
    try {
      const peerGroups = await fetchJSON('/api/amnesia/by-peer');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No amnesia events</div>';
        return;
      }
      let html = '';
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];
        html += `<div class="tree-node peer-header clickable" data-amn-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-amn-peer-children="${pi}">`;

        // Group events by session
        const sessionMap = {};
        const sessionOrder = [];
        for (const a of pg.events) {
          const sid = a.session_id || 'unknown';
          if (!sessionMap[sid]) {
            sessionMap[sid] = [];
            sessionOrder.push(sid);
          }
          sessionMap[sid].push(a);
        }

        for (let si = 0; si < sessionOrder.length; si++) {
          const sid = sessionOrder[si];
          const sessionEvents = sessionMap[sid];
          const shortSid = sid.length > 12 ? sid.substring(0, 12) : sid;
          const taskId = sessionEvents[0].task_id;
          const shortTask = taskId ? taskId.substring(0, 8) : '';

          html += `<div class="tree-node clickable" style="padding-left:24px" data-amn-session="${pi}-${si}">
            <span class="tree-arrow">&#x25B6;</span>
            <span class="session-uuid">${esc(shortSid)}</span>
            ${shortTask ? `<span class="badge scope" style="font-size:10px">${esc(shortTask)}</span>` : ''}
            <span class="count">${sessionEvents.length}</span>
          </div>`;
          html += `<div style="display:none" data-amn-session-children="${pi}-${si}">`;

          for (const a of sessionEvents) {
            const scorePercent = Math.round(a.score * 100);
            const scoreClass = scorePercent >= 80 ? 'high' : scorePercent >= 60 ? 'medium' : 'low';
            const statusClass = a.status === 'confirmed' ? 'confirmed' : a.status === 'dismissed' ? 'dismissed' : 'flagged';

            html += `<div class="amnesia-item ${statusClass}" style="margin-left:24px">
              <div class="amnesia-header">
                <span class="amnesia-score ${scoreClass}">${scorePercent}%</span>
                <span class="badge type">${esc(a.tool_name)}</span>
                ${a.action_id ? `<span class="badge scope action-link" style="cursor:pointer;font-size:10px" data-aid="${esc(a.action_id)}">view action</span>` : ''}
                <span class="amnesia-status-label">${esc(a.status)}</span>
                <span style="color:var(--text-muted);font-size:11px;margin-left:auto">${formatTime(a.created_at)}</span>
              </div>
              <div class="amnesia-rule">
                <span style="color:var(--red);font-weight:500">Rule [${esc(a.rule_marker)}]:</span> ${esc(a.rule_text)}
              </div>
              <div class="amnesia-action">
                <span style="color:var(--text-muted)">Action:</span> ${esc(a.tool_input)}
              </div>
              ${a.status === 'flagged' ? `<div class="amnesia-actions">
                <button class="btn-confirm" onclick="window._amnesiaAction(${a.id},'confirm')">Confirm Violation</button>
                <button class="btn-dismiss" onclick="window._amnesiaAction(${a.id},'dismiss')">Dismiss</button>
              </div>` : ''}
            </div>`;
          }
          html += `</div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      // Peer toggle
      el.querySelectorAll('[data-amn-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.amnPeer;
          const children = el.querySelector(`[data-amn-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Session toggle
      el.querySelectorAll('[data-amn-session]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.amnSession;
          const children = el.querySelector(`[data-amn-session-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Action cross-links
      el.querySelectorAll('.action-link').forEach(link => {
        link.addEventListener('click', () => {
          switchView('actions');
          setTimeout(() => loadActionDetail(link.dataset.aid), 300);
        });
      });
    } catch (e) {
      console.error('Load amnesia failed:', e);
    }
  }

  window._amnesiaAction = async function(id, action) {
    await fetch('/api/amnesia/' + id + '/' + action, {method: 'POST'});
    loadAmnesia();
  };

  // Update amnesia badge in sidebar
  async function updateAmnesiaCount() {
    try {
      const events = await fetchJSON('/api/amnesia?status=flagged');
      const badge = document.getElementById('amnesia-badge');
      if (events && events.length > 0) {
        badge.textContent = events.length;
        badge.style.display = '';
      } else {
        badge.style.display = 'none';
      }
    } catch (e) {}
  }

  // --- Visceral ---
  async function loadVisceral() {
    const el = document.getElementById('visceral-rules');
    const input = document.getElementById('visceral-input');
    const btn = document.getElementById('visceral-add-btn');

    btn.onclick = async () => {
      const rule = input.value.trim();
      if (!rule) return;
      await fetch('/api/visceral', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({rule})
      });
      input.value = '';
      loadVisceral();
    };
    input.onkeypress = (e) => { if (e.key === 'Enter') btn.click(); };

    try {
      const peerGroups = await fetchJSON('/api/visceral/by-peer');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No visceral rules set yet. Add rules that every agent must follow.</div>';
        return;
      }
      let html = '';
      let ruleNum = 0;
      for (let pi = 0; pi < peerGroups.length; pi++) {
        const pg = peerGroups[pi];
        html += `<div class="tree-node peer-header clickable" data-visc-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-visc-peer-children="${pi}">`;
        for (const r of pg.rules) {
          ruleNum++;
          html += `<div class="visceral-rule" style="padding-left:24px">
            <span class="visceral-num">${ruleNum}</span>
            <span class="session-uuid">${esc(r.marker)}</span>
            <span class="visceral-text">${esc(r.text)}</span>
            <span style="color:var(--text-muted);font-size:11px">${esc(r.source || '')}</span>
            <button class="btn-copy" onclick="window._copy('recall memory ${esc(r.marker)}')" title="Copy reference">&#x2398;</button>
            <button class="visceral-delete" onclick="window._deleteVisceral('${esc(r.id)}')" title="Remove rule">&times;</button>
          </div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      el.querySelectorAll('[data-visc-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.viscPeer;
          const children = el.querySelector(`[data-visc-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });
    } catch (e) {
      console.error('Load visceral failed:', e);
    }
  }

  window._deleteVisceral = async function(id) {
    if (!confirm('Remove this visceral rule?')) return;
    await fetch('/api/visceral/' + id, {method: 'DELETE'});
    loadVisceral();
  };

  // --- Recall ---
  async function loadRecall() {
    const el = document.getElementById('recall-list');
    try {
      const items = await fetchJSON('/api/recall');
      if (!items || !items.length) {
        el.innerHTML = '<div class="empty-state">No deleted items. Everything is still alive.</div>';
        return;
      }

      // Group by type
      const groups = {};
      for (const item of items) {
        if (!groups[item.type]) groups[item.type] = [];
        groups[item.type].push(item);
      }

      const typeIcons = {memory: '&#x25A1;', manifest: '&#x2637;', idea: '&#x2726;', task: '&#x23F0;'};
      const typeColors = {memory: 'var(--accent)', manifest: 'var(--green)', idea: 'var(--yellow)', task: 'var(--text-secondary)'};

      let html = '';
      for (const type of ['memory', 'manifest', 'idea', 'task']) {
        const list = groups[type];
        if (!list || !list.length) continue;
        html += `<div style="margin-bottom:16px">
          <div style="font-size:13px;font-weight:600;color:${typeColors[type]};margin-bottom:8px;text-transform:capitalize">${typeIcons[type]} ${type}s (${list.length})</div>`;
        for (const item of list) {
          html += `<div style="display:flex;align-items:center;gap:8px;padding:8px 0;border-bottom:1px solid var(--border)">
            <span class="session-uuid">${esc(item.marker)}</span>
            <span style="font-size:13px;color:var(--text-primary);flex:1">${esc(item.title)}</span>
            <button class="btn-search recall-restore" data-type="${esc(item.type)}" data-id="${esc(item.id)}" style="font-size:11px;padding:4px 10px">Restore</button>
          </div>`;
        }
        html += `</div>`;
      }
      el.innerHTML = html;

      el.querySelectorAll('.recall-restore').forEach(btn => {
        btn.addEventListener('click', async () => {
          const type = btn.dataset.type;
          const id = btn.dataset.id;
          await fetch('/api/recall/' + type + '/' + id + '/restore', {method: 'POST'});
          btn.textContent = 'Restored';
          btn.disabled = true;
          btn.style.background = 'var(--green)';
          setTimeout(() => loadRecall(), 1000);
        });
      });
    } catch (e) {
      console.error('Load recall failed:', e);
    }
  }

  // --- Settings ---
  async function loadSettings() {
    // Show MCP config snippet
    const snippet = document.getElementById('mcp-config-snippet');
    if (snippet) {
      snippet.textContent = JSON.stringify({
        "mcpServers": {
          "openloom": {
            "type": "http",
            "url": "http://127.0.0.1:8765/mcp"
          }
        }
      }, null, 2);
    }

    // Load profile
    try {
      const profile = await fetchJSON('/api/settings/profile');
      document.getElementById('profile-uuid').textContent = profile.uuid || '';
      document.getElementById('profile-name').value = profile.display_name || '';
      document.getElementById('profile-email').value = profile.email || '';
      document.getElementById('profile-avatar').value = profile.avatar || '';
      updateProfilePreview(profile);
    } catch (e) {}

    // Emoji picker
    const emojis = [
      '😀','😎','🤖','👻','🦊','🐺','🦁','🐯','🐻','🐼',
      '🐨','🐸','🐙','🦉','🦅','🐝','🦋','🐲','🔥','⚡',
      '🌊','🌈','💎','🚀','🛸','🎯','🎮','🧠','💻','🔮',
      '⭐','🌙','☀️','🍀','🌺','🏔️','🌍','🎵','🎨','🛡️',
      '⚔️','🔱','👑','🎩','🧊','🌀','💜','💙','💚','🧡',
    ];
    const pickerEl = document.getElementById('emoji-picker');
    const avatarInput = document.getElementById('profile-avatar');
    const previewSm = document.getElementById('profile-avatar-preview-sm');
    pickerEl.innerHTML = emojis.map(e =>
      `<span class="emoji-btn" style="font-size:22px;cursor:pointer;padding:4px 6px;border-radius:4px;transition:background 0.1s">${e}</span>`
    ).join('');
    pickerEl.querySelectorAll('.emoji-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        avatarInput.value = btn.textContent;
        previewSm.textContent = btn.textContent;
        updateProfilePreview({display_name: document.getElementById('profile-name').value, email: document.getElementById('profile-email').value, avatar: btn.textContent});
      });
      btn.addEventListener('mouseenter', () => btn.style.background = 'var(--bg-card-hover)');
      btn.addEventListener('mouseleave', () => btn.style.background = '');
    });
    avatarInput.addEventListener('input', () => {
      previewSm.textContent = avatarInput.value.startsWith('http') ? '' : avatarInput.value;
    });
    previewSm.textContent = avatarInput.value.startsWith('http') ? '' : avatarInput.value;

    // Wire save button
    const saveBtn = document.getElementById('profile-save-btn');
    saveBtn.onclick = async () => {
      const body = {
        display_name: document.getElementById('profile-name').value.trim(),
        email: document.getElementById('profile-email').value.trim(),
        avatar: document.getElementById('profile-avatar').value.trim(),
      };
      try {
        await fetch('/api/settings/profile', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify(body)
        });
        const status = document.getElementById('profile-save-status');
        status.textContent = 'Saved';
        setTimeout(() => status.textContent = '', 2000);
        updateProfilePreview(body);
      } catch (e) {
        console.error('Save profile failed:', e);
      }
    };

    // Load chat provider settings
    try {
      const chatSettings = await fetchJSON('/api/settings/chat');
      renderChatProviderSettings(chatSettings);
    } catch (e) {
      console.error('Load chat settings failed:', e);
    }

    // Load detected agents
    try {
      const agents = await fetchJSON('/api/settings/agents');
      renderAgentSettings(agents || []);
    } catch (e) {
      console.error('Load settings failed:', e);
    }
  }

  function renderChatProviderSettings(settings) {
    if (!settings || !settings.providers) return;

    const providers = settings.providers;

    // Update status indicators
    for (const [name, info] of Object.entries(providers)) {
      const statusEl = document.getElementById('status-' + name);
      if (!statusEl) continue;

      if (info.has_key) {
        statusEl.innerHTML = '&#x2705;'; // green check
        statusEl.title = info.from_env ? 'Key from environment variable' : 'Key configured';
      } else if (name === 'ollama') {
        statusEl.innerHTML = '&#x25CB;'; // circle
        statusEl.title = 'Test connection to verify';
      } else {
        statusEl.innerHTML = '&#x274C;'; // red X
        statusEl.title = 'No API key configured';
      }

      // Show placeholder hint for env-sourced keys
      const input = document.getElementById('key-' + name);
      if (input && info.from_env && name !== 'ollama') {
        input.placeholder = 'Using ' + (info.env_var || 'env var') + ' (set to override)';
      }
      if (input && name === 'ollama' && info.host) {
        input.value = info.host;
      }
    }

    // Wire test buttons
    document.querySelectorAll('.provider-test-btn').forEach(btn => {
      btn.onclick = async () => {
        const provider = btn.dataset.provider;
        const input = document.getElementById('key-' + provider);
        const statusEl = document.getElementById('status-' + provider);
        btn.disabled = true;
        btn.textContent = '...';
        statusEl.innerHTML = '&#x23F3;'; // hourglass
        statusEl.title = 'Testing...';

        try {
          const body = {provider};
          if (provider === 'ollama') {
            body.host = input.value || 'http://localhost:11434';
          } else {
            body.api_key = input.value;
          }
          const resp = await fetch('/api/settings/chat/test', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(body),
          });
          const result = await resp.json();
          if (result.valid) {
            statusEl.innerHTML = '&#x2705;';
            statusEl.title = 'Connection valid';
          } else {
            statusEl.innerHTML = '&#x274C;';
            statusEl.title = result.error || 'Test failed';
          }
        } catch (e) {
          statusEl.innerHTML = '&#x274C;';
          statusEl.title = 'Test failed: ' + e.message;
        }
        btn.disabled = false;
        btn.textContent = 'Test';
      };
    });

    // Wire save buttons
    document.querySelectorAll('.provider-save-btn').forEach(btn => {
      btn.onclick = async () => {
        const provider = btn.dataset.provider;
        const input = document.getElementById('key-' + provider);
        if (!input.value) return;

        btn.disabled = true;
        btn.textContent = '...';

        try {
          const body = {};
          body[provider + '_key'] = input.value;
          await fetch('/api/settings/chat', {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(body),
          });
          btn.textContent = 'Saved';
          setTimeout(() => { btn.textContent = 'Save'; }, 1500);

          // Refresh models in chat view
          const modelData = await fetchJSON('/api/chat/models');
          chatModels = modelData.models || [];
          populateModelSelect();

          // Re-fetch settings to update status
          const chatSettings = await fetchJSON('/api/settings/chat');
          renderChatProviderSettings(chatSettings);
        } catch (e) {
          btn.textContent = 'Error';
          setTimeout(() => { btn.textContent = 'Save'; }, 1500);
        }
        btn.disabled = false;
      };
    });
  }

  function updateProfilePreview(profile) {
    const avatarEl = document.getElementById('profile-avatar-display');
    const nameEl = document.getElementById('profile-display-preview');
    const emailEl = document.getElementById('profile-email-preview');

    const avatar = profile.avatar || profile.display_name?.charAt(0)?.toUpperCase() || '?';
    if (avatar.startsWith('http')) {
      avatarEl.innerHTML = `<img src="${esc(avatar)}" style="width:56px;height:56px;border-radius:50%;object-fit:cover" />`;
    } else {
      avatarEl.textContent = avatar;
    }
    nameEl.textContent = profile.display_name || 'Unnamed Node';
    emailEl.textContent = profile.email || '';
  }

  function renderAgentSettings(agents) {
    const el = document.getElementById('settings-agents');
    if (!agents.length) {
      el.innerHTML = '<div class="empty-state">No agents detected</div>';
      return;
    }

    el.innerHTML = agents.map(a => {
      let statusClass = 'not-installed';
      let statusText = 'Not installed';
      let button = '';

      if (a.installed && a.connected) {
        statusClass = 'connected';
        statusText = 'Connected';
        button = `<button class="btn-disconnect" onclick="window._disconnectAgent('${esc(a.id)}')">Disconnect</button>`;
      } else if (a.installed && !a.connected) {
        statusClass = 'disconnected';
        statusText = 'Not connected';
        button = `<button class="btn-connect" onclick="window._connectAgent('${esc(a.id)}')">Connect</button>`;
      } else {
        statusText = 'Not installed';
      }

      return `<div class="agent-card">
        <div class="agent-card-info">
          <span class="status-dot ${a.connected ? 'green' : a.installed ? 'yellow' : 'red'}"></span>
          <div>
            <div class="agent-card-name">${esc(a.name)}</div>
            <div class="agent-card-status ${statusClass}">${statusText}</div>
            <div class="agent-card-path">${esc(a.config_path)}</div>
          </div>
        </div>
        ${button}
      </div>`;
    }).join('');
  }

  window._connectAgent = async function(id) {
    await fetch('/api/settings/agents/' + id + '/connect', {method: 'POST'});
    loadSettings();
  };

  window._disconnectAgent = async function(id) {
    await fetch('/api/settings/agents/' + id + '/disconnect', {method: 'POST'});
    loadSettings();
  };

  // --- Chat ---
  let chatSessions = [];
  let chatActiveSessionId = null;
  let chatModels = [];
  let chatAbortController = null;
  let chatStreaming = false;
  let chatAttachments = [];

  async function loadChat() {
    try {
      const [sessions, modelData] = await Promise.all([
        fetchJSON('/api/chat/sessions'),
        fetchJSON('/api/chat/models'),
      ]);
      chatSessions = sessions || [];
      chatModels = modelData.models || [];
      populateModelSelect();
      renderChatTabs();
      if (chatSessions.length === 0) {
        await createChatSession();
      } else if (!chatActiveSessionId || !chatSessions.find(s => s.id === chatActiveSessionId)) {
        switchChatTab(chatSessions[0].id);
      } else {
        switchChatTab(chatActiveSessionId);
      }
    } catch (e) {
      console.error('Load chat failed:', e);
    }
  }

  const providerColors = {
    anthropic: '#d97706',
    google: '#4285f4',
    openai: '#10a37f',
    ollama: '#8b5cf6',
  };

  const providerLabels = {
    anthropic: 'Anthropic',
    google: 'Google',
    openai: 'OpenAI',
    ollama: 'Ollama',
  };

  function populateModelSelect() {
    const sel = document.getElementById('chat-model-select');
    if (!sel) return;

    // Group by provider
    const groups = {};
    for (const m of chatModels) {
      if (!groups[m.provider]) groups[m.provider] = [];
      groups[m.provider].push(m);
    }

    let html = '';
    const order = ['anthropic', 'google', 'openai', 'ollama'];
    for (const prov of order) {
      const models = groups[prov];
      if (!models || models.length === 0) continue;
      const label = providerLabels[prov] || prov;
      html += `<optgroup label="${esc(label)}">`;
      for (const m of models) {
        const cost = m.cost_per_m_in > 0 ? ` $${m.cost_per_m_in}/$${m.cost_per_m_out}` : ' free';
        html += `<option value="${esc(m.id)}">${esc(m.name)}${cost}</option>`;
      }
      html += '</optgroup>';
    }
    sel.innerHTML = html;
  }

  function renderChatTabs() {
    const container = document.getElementById('chat-tabs');
    if (!container) return;
    container.innerHTML = chatSessions.map(s => {
      const active = s.id === chatActiveSessionId ? 'background:var(--bg-card-hover);color:var(--text);border-bottom:2px solid var(--accent)' : 'color:var(--text-muted)';
      const title = s.title || 'New Chat';
      const shortTitle = title.length > 25 ? title.substring(0, 25) + '...' : title;
      return `<div class="chat-tab" data-id="${esc(s.id)}" style="display:flex;align-items:center;gap:4px;padding:6px 12px;cursor:pointer;font-size:12px;white-space:nowrap;border-radius:4px 4px 0 0;${active};flex-shrink:0;position:relative" title="${esc(title)}">
        <span class="chat-tab-title" style="max-width:150px;overflow:hidden;text-overflow:ellipsis">${esc(shortTitle)}</span>
        <span class="chat-tab-close" style="font-size:14px;opacity:0.5;cursor:pointer;padding:0 2px" title="Close">&times;</span>
      </div>`;
    }).join('');

    // Tab click handlers
    container.querySelectorAll('.chat-tab').forEach(tab => {
      tab.addEventListener('click', (e) => {
        if (e.target.classList.contains('chat-tab-close')) return;
        switchChatTab(tab.dataset.id);
      });
      tab.querySelector('.chat-tab-close').addEventListener('click', (e) => {
        e.stopPropagation();
        deleteChatSession(tab.dataset.id);
      });
      // Double-click to rename
      tab.querySelector('.chat-tab-title').addEventListener('dblclick', (e) => {
        e.stopPropagation();
        const titleEl = e.target;
        const id = tab.dataset.id;
        const sess = chatSessions.find(s => s.id === id);
        if (!sess) return;
        const input = document.createElement('input');
        input.type = 'text';
        input.value = sess.title;
        input.style.cssText = 'font-size:12px;width:120px;padding:1px 4px;border:1px solid var(--accent);border-radius:3px;background:var(--bg-input);color:var(--text)';
        titleEl.replaceWith(input);
        input.focus();
        input.select();
        const finish = async () => {
          const newTitle = input.value.trim() || sess.title;
          await fetch('/api/chat/sessions/' + id + '/title', {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({title: newTitle}),
          });
          sess.title = newTitle;
          renderChatTabs();
        };
        input.addEventListener('blur', finish);
        input.addEventListener('keydown', (ev) => { if (ev.key === 'Enter') { ev.preventDefault(); finish(); }});
      });
    });
  }

  function switchChatTab(id) {
    chatActiveSessionId = id;
    renderChatTabs();
    const sess = chatSessions.find(s => s.id === id);
    if (!sess) return;
    // Update model/thinking selects
    const modelSel = document.getElementById('chat-model-select');
    if (modelSel) modelSel.value = sess.model;
    const thinkSel = document.getElementById('chat-thinking-select');
    if (thinkSel) thinkSel.value = sess.thinking_level || 'off';
    // Update model provider indicator
    updateModelIndicator(sess.model);
    // Update usage display
    updateChatUsage(sess);
    // Render messages
    renderChatMessages(sess.messages || []);
  }

  function updateChatUsage(sess) {
    const el = document.getElementById('chat-usage');
    if (el && sess) {
      el.textContent = `${sess.tokens_in + sess.tokens_out} tokens | $${sess.cost_usd.toFixed(4)}`;
    }
  }

  function updateModelIndicator(modelId) {
    const indicator = document.getElementById('chat-model-indicator');
    if (!indicator || !modelId) return;
    const provider = modelId.split('/')[0];
    const color = providerColors[provider] || 'var(--text-muted)';
    indicator.style.background = color;
    indicator.title = providerLabels[provider] || provider;
  }

  function renderChatMessages(messages) {
    const container = document.getElementById('chat-messages');
    if (!container) return;
    if (!messages || messages.length === 0) {
      container.innerHTML = '<div class="empty-state" style="margin:auto;text-align:center;color:var(--text-muted)"><div style="font-size:32px;margin-bottom:12px">&#x1F4AC;</div><div style="font-size:15px;font-weight:500">Start a conversation</div><div style="font-size:13px;margin-top:6px;color:var(--text-muted)">Ask about memories, manifests, tasks, or anything else</div></div>';
      return;
    }
    container.innerHTML = messages.map((msg, i) => {
      if (msg.role === 'tool') return ''; // tool results shown inline with tool_calls
      const isUser = msg.role === 'user';
      const bgColor = isUser ? 'var(--bg-secondary)' : 'transparent';
      const label = isUser ? 'You' : 'Assistant';
      const labelColor = isUser ? 'var(--accent)' : 'var(--green)';
      const borderLeft = isUser ? '' : 'border-left:2px solid var(--green);padding-left:14px;';

      let content = formatChatContent(msg.content);

      // Show tool calls if present
      let toolCallsHtml = '';
      if (msg.tool_calls && msg.tool_calls.length > 0) {
        toolCallsHtml = '<div style="margin-top:10px;display:flex;flex-direction:column;gap:6px">' + msg.tool_calls.map(tc => {
          const nextMsg = messages[i + 1];
          const result = (nextMsg && nextMsg.role === 'tool' && nextMsg.tool_call_id === tc.id) ? nextMsg.content : '';
          return `<details style="background:var(--bg-secondary);border-radius:6px;border-left:3px solid var(--yellow);font-size:12px">
            <summary style="padding:8px 12px;cursor:pointer;display:flex;align-items:center;gap:6px">
              <span style="color:var(--yellow);font-weight:600">&#x2699; ${esc(tc.name)}</span>
              <span style="color:var(--text-muted);font-family:var(--font-mono);font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:300px">${esc(tc.input ? tc.input.substring(0, 80) : '')}</span>
            </summary>
            ${result ? `<div style="padding:8px 12px;border-top:1px solid var(--border);color:var(--text-secondary);white-space:pre-wrap;font-family:var(--font-mono);font-size:11px;max-height:150px;overflow-y:auto">${esc(result)}</div>` : ''}
          </details>`;
        }).join('') + '</div>';
      }

      return `<div style="padding:14px 18px;border-radius:10px;background:${bgColor};${borderLeft}">
        <div style="font-size:11px;font-weight:700;color:${labelColor};margin-bottom:8px;text-transform:uppercase;letter-spacing:0.5px">${label}</div>
        <div class="chat-msg-content" style="font-size:15px;line-height:1.7;color:var(--text)">${content}</div>
        ${toolCallsHtml}
      </div>`;
    }).filter(Boolean).join('');
    container.scrollTop = container.scrollHeight;
  }

  function formatChatContent(text) {
    if (!text) return '';
    let html = esc(text);

    // Code blocks with language label
    html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (match, lang, code) => {
      const langLabel = lang ? `<div style="font-size:10px;color:var(--text-muted);padding:4px 12px 0;text-transform:uppercase;letter-spacing:0.5px">${lang}</div>` : '';
      return `<div style="background:var(--bg-secondary);border-radius:8px;margin:10px 0;overflow:hidden;border:1px solid var(--border)">${langLabel}<pre style="padding:10px 14px;overflow-x:auto;font-size:13px;line-height:1.5;margin:0"><code>${code}</code></pre></div>`;
    });

    // Inline code
    html = html.replace(/`([^`]+)`/g, '<code style="background:var(--bg-secondary);padding:2px 6px;border-radius:4px;font-size:13px;border:1px solid var(--border)">$1</code>');

    // Headers (## and ###)
    html = html.replace(/^### (.+)$/gm, '<div style="font-size:14px;font-weight:700;margin:12px 0 6px;color:var(--text)">$1</div>');
    html = html.replace(/^## (.+)$/gm, '<div style="font-size:15px;font-weight:700;margin:14px 0 8px;color:var(--text)">$1</div>');

    // Bullet lists
    html = html.replace(/^[*-] (.+)$/gm, '<div style="padding-left:16px;position:relative"><span style="position:absolute;left:4px;color:var(--text-muted)">&#x2022;</span>$1</div>');

    // Numbered lists
    html = html.replace(/^(\d+)\. (.+)$/gm, '<div style="padding-left:20px;position:relative"><span style="position:absolute;left:0;color:var(--text-muted);font-size:13px">$1.</span>$2</div>');

    // Bold
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    // Italic
    html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');

    // Links
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" style="color:var(--accent);text-decoration:underline">$1</a>');

    // Wrap in white-space:pre-wrap for line breaks
    return '<div style="white-space:pre-wrap;word-wrap:break-word">' + html + '</div>';
  }

  async function createChatSession() {
    try {
      const resp = await fetch('/api/chat/sessions', {method: 'POST'});
      const sess = await resp.json();
      chatSessions.unshift(sess);
      switchChatTab(sess.id);
      renderChatTabs();
    } catch (e) {
      console.error('Create session failed:', e);
    }
  }

  async function deleteChatSession(id) {
    try {
      await fetch('/api/chat/sessions/' + id, {method: 'DELETE'});
      chatSessions = chatSessions.filter(s => s.id !== id);
      if (chatActiveSessionId === id) {
        if (chatSessions.length > 0) {
          switchChatTab(chatSessions[0].id);
        } else {
          chatActiveSessionId = null;
          await createChatSession();
        }
      }
      renderChatTabs();
    } catch (e) {}
  }

  async function sendChatMessage() {
    const input = document.getElementById('chat-input');
    const message = input.value.trim();
    if (!message || !chatActiveSessionId || chatStreaming) return;

    input.value = '';
    input.style.height = 'auto';
    chatStreaming = true;

    // Show send/abort toggle
    document.getElementById('chat-send-btn').style.display = 'none';
    document.getElementById('chat-abort-btn').style.display = '';

    // Add user message to UI immediately
    const sess = chatSessions.find(s => s.id === chatActiveSessionId);
    if (!sess.messages) sess.messages = [];
    const userMsg = {role: 'user', content: message, attachments: chatAttachments.length > 0 ? [...chatAttachments] : undefined};
    sess.messages.push(userMsg);
    renderChatMessages(sess.messages);

    // Prepare attachments
    const attachments = chatAttachments.map(a => ({type: 'image', mime_type: a.mime_type, base64: a.base64}));
    chatAttachments = [];
    const preview = document.getElementById('chat-attachments-preview');
    if (preview) { preview.style.display = 'none'; preview.innerHTML = ''; }

    // Stream response
    chatAbortController = new AbortController();
    try {
      const resp = await fetch('/api/chat', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({session_id: chatActiveSessionId, message, attachments}),
        signal: chatAbortController.signal,
      });

      const reader = resp.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let assistantText = '';
      let toolCalls = [];

      // Add placeholder assistant message
      const assistantMsg = {role: 'assistant', content: ''};
      sess.messages.push(assistantMsg);

      while (true) {
        const {done, value} = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, {stream: true});

        const lines = buffer.split('\n');
        buffer = lines.pop(); // Keep incomplete line in buffer

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;
          const data = line.substring(6);
          try {
            const chunk = JSON.parse(data);
            switch (chunk.type) {
              case 'text':
                assistantText += chunk.content;
                assistantMsg.content = assistantText;
                renderChatMessages(sess.messages);
                break;
              case 'thinking':
                // Show thinking inline (greyed out)
                assistantMsg.content = assistantText + '\n[thinking: ' + (chunk.content || '').substring(0, 100) + '...]';
                renderChatMessages(sess.messages);
                break;
              case 'tool_call':
                if (chunk.tool_call) {
                  toolCalls.push(chunk.tool_call);
                  if (!assistantMsg.tool_calls) assistantMsg.tool_calls = [];
                  assistantMsg.tool_calls.push(chunk.tool_call);
                  renderChatMessages(sess.messages);
                }
                break;
              case 'tool_result':
                if (chunk.tool_result) {
                  sess.messages.push({role: 'tool', content: chunk.tool_result.result, tool_call_id: chunk.tool_result.id});
                  renderChatMessages(sess.messages);
                }
                break;
              case 'done':
                assistantMsg.content = assistantText;
                if (chunk.usage) {
                  sess.tokens_in = (sess.tokens_in || 0) + chunk.usage.input_tokens;
                  sess.tokens_out = (sess.tokens_out || 0) + chunk.usage.output_tokens;
                }
                if (chunk.cost) {
                  sess.cost_usd = (sess.cost_usd || 0) + chunk.cost;
                }
                updateChatUsage(sess);
                break;
              case 'error':
                assistantMsg.content = assistantText + '\n\n[Error: ' + (chunk.error || 'Unknown') + ']';
                renderChatMessages(sess.messages);
                break;
            }
          } catch (e) {}
        }
      }

      // Final render
      assistantMsg.content = assistantText;
      renderChatMessages(sess.messages);

      // Auto-update title if this was the first message
      if (sess.title === 'New Chat' && message.length > 0) {
        sess.title = message.length > 50 ? message.substring(0, 50) + '...' : message;
        renderChatTabs();
      }

    } catch (e) {
      if (e.name !== 'AbortError') {
        console.error('Chat stream failed:', e);
        sess.messages.push({role: 'assistant', content: '[Connection error: ' + e.message + ']'});
        renderChatMessages(sess.messages);
      }
    }

    chatStreaming = false;
    chatAbortController = null;
    document.getElementById('chat-send-btn').style.display = '';
    document.getElementById('chat-abort-btn').style.display = 'none';
  }

  function abortChat() {
    if (chatAbortController) {
      chatAbortController.abort();
    }
  }

  // Chat event handlers (wired once on DOMContentLoaded)
  document.addEventListener('DOMContentLoaded', () => {
    const sendBtn = document.getElementById('chat-send-btn');
    const abortBtn = document.getElementById('chat-abort-btn');
    const newTabBtn = document.getElementById('chat-new-tab');
    const chatInput = document.getElementById('chat-input');
    const modelSelect = document.getElementById('chat-model-select');
    const thinkSelect = document.getElementById('chat-thinking-select');
    const fileInput = document.getElementById('chat-file-input');

    if (sendBtn) sendBtn.addEventListener('click', sendChatMessage);
    if (abortBtn) abortBtn.addEventListener('click', abortChat);
    if (newTabBtn) newTabBtn.addEventListener('click', createChatSession);

    if (chatInput) {
      chatInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          sendChatMessage();
        }
      });
      // Auto-resize textarea
      chatInput.addEventListener('input', () => {
        chatInput.style.height = 'auto';
        chatInput.style.height = Math.min(chatInput.scrollHeight, 150) + 'px';
      });
    }

    if (modelSelect) {
      modelSelect.addEventListener('change', async () => {
        if (!chatActiveSessionId) return;
        const model = modelSelect.value;
        await fetch('/api/chat/sessions/' + chatActiveSessionId + '/model', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({model}),
        });
        const sess = chatSessions.find(s => s.id === chatActiveSessionId);
        if (sess) sess.model = model;
        updateModelIndicator(model);
      });
    }

    if (thinkSelect) {
      thinkSelect.addEventListener('change', async () => {
        if (!chatActiveSessionId) return;
        const level = thinkSelect.value;
        await fetch('/api/chat/sessions/' + chatActiveSessionId + '/thinking', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({level}),
        });
        const sess = chatSessions.find(s => s.id === chatActiveSessionId);
        if (sess) sess.thinking_level = level;
      });
    }

    // File attachments
    if (fileInput) {
      fileInput.addEventListener('change', async () => {
        const files = Array.from(fileInput.files);
        for (const file of files) {
          if (!file.type.startsWith('image/')) continue;
          const base64 = await fileToBase64(file);
          chatAttachments.push({mime_type: file.type, base64, name: file.name});
        }
        renderAttachmentPreviews();
        fileInput.value = '';
      });
    }

    // Keyboard shortcuts
    document.addEventListener('keydown', (e) => {
      if (currentView !== 'chat') return;
      // Cmd+T — new tab
      if ((e.metaKey || e.ctrlKey) && e.key === 't') {
        e.preventDefault();
        createChatSession();
      }
      // Cmd+W — close tab
      if ((e.metaKey || e.ctrlKey) && e.key === 'w') {
        e.preventDefault();
        if (chatActiveSessionId) deleteChatSession(chatActiveSessionId);
      }
      // Cmd+1-9 — switch tab by number
      if ((e.metaKey || e.ctrlKey) && e.key >= '1' && e.key <= '9') {
        e.preventDefault();
        const idx = parseInt(e.key) - 1;
        if (idx < chatSessions.length) switchChatTab(chatSessions[idx].id);
      }
      // Cmd+Shift+] / Cmd+Shift+[ — next/prev tab
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === ']') {
        e.preventDefault();
        const idx = chatSessions.findIndex(s => s.id === chatActiveSessionId);
        if (idx < chatSessions.length - 1) switchChatTab(chatSessions[idx + 1].id);
      }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === '[') {
        e.preventDefault();
        const idx = chatSessions.findIndex(s => s.id === chatActiveSessionId);
        if (idx > 0) switchChatTab(chatSessions[idx - 1].id);
      }
    });
  });

  function fileToBase64(file) {
    return new Promise((resolve) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result.split(',')[1]);
      reader.readAsDataURL(file);
    });
  }

  function renderAttachmentPreviews() {
    const container = document.getElementById('chat-attachments-preview');
    if (!container) return;
    if (chatAttachments.length === 0) {
      container.style.display = 'none';
      return;
    }
    container.style.display = 'flex';
    container.innerHTML = chatAttachments.map((a, i) =>
      `<div style="position:relative;width:48px;height:48px;border-radius:6px;overflow:hidden;border:1px solid var(--border)">
        <img src="data:${esc(a.mime_type)};base64,${a.base64}" style="width:100%;height:100%;object-fit:cover" />
        <span class="chat-remove-attach" data-idx="${i}" style="position:absolute;top:-2px;right:2px;cursor:pointer;font-size:14px;color:var(--red,#e74c3c)">&times;</span>
      </div>`
    ).join('');
    container.querySelectorAll('.chat-remove-attach').forEach(btn => {
      btn.addEventListener('click', () => {
        chatAttachments.splice(parseInt(btn.dataset.idx), 1);
        renderAttachmentPreviews();
      });
    });
  }

  // Expose switchView to global for inline onclick handlers
  window.switchView = switchView;

  // --- Helpers ---
  async function fetchJSON(url, opts) {
    const resp = await fetch(url, opts);
    if (!resp.ok && resp.status !== 200) {
      const text = await resp.text();
      throw new Error(text || resp.statusText);
    }
    const text = await resp.text();
    if (!text) return null;
    return JSON.parse(text);
  }

  function setText(id, text) {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
  }

  function esc(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  function formatModel(model) {
    if (!model) return '';
    // claude-opus-4-6 → Claude Opus 4.6
    // claude-sonnet-4-6 → Claude Sonnet 4.6
    const parts = model.split('-');
    if (parts[0] === 'claude' && parts.length >= 3) {
      const family = parts[1].charAt(0).toUpperCase() + parts[1].slice(1);
      const version = parts.slice(2).join('.');
      return `Claude ${family} ${version}`;
    }
    // gpt-4o → GPT-4o
    if (model.startsWith('gpt')) return model.toUpperCase();
    // gemini-2.5-pro → Gemini 2.5 Pro
    if (model.startsWith('gemini')) {
      return model.split('-').map(p => p.charAt(0).toUpperCase() + p.slice(1)).join(' ');
    }
    return model;
  }

  function timeAgo(iso) {
    if (!iso) return '';
    const diff = Date.now() - new Date(iso).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
  }

  // --- Watcher ---

  async function loadWatcher() {
    try {
      const [audits, stats] = await Promise.all([
        fetchJSON('/api/watcher/audits'),
        fetchJSON('/api/watcher/stats'),
      ]);

      // Stats bar
      const statsBar = document.getElementById('watcher-stats-bar');
      if (stats) {
        const passColor = stats.pass_rate >= 80 ? 'var(--green)' : stats.pass_rate >= 50 ? 'var(--yellow)' : 'var(--red)';
        statsBar.innerHTML = `
          <div class="metric-card">
            <div class="metric-value">${stats.total_audits}</div>
            <div class="metric-label">Total Audits</div>
          </div>
          <div class="metric-card">
            <div class="metric-value" style="color:var(--green)">${stats.passed}</div>
            <div class="metric-label">Passed</div>
          </div>
          <div class="metric-card">
            <div class="metric-value" style="color:var(--red)">${stats.failed}</div>
            <div class="metric-label">Failed</div>
          </div>
          <div class="metric-card">
            <div class="metric-value" style="color:var(--yellow)">${stats.warnings}</div>
            <div class="metric-label">Warnings</div>
          </div>
          <div class="metric-card">
            <div class="metric-value" style="color:${passColor}">${stats.pass_rate.toFixed(0)}%</div>
            <div class="metric-label">Pass Rate</div>
          </div>
        `;
      }

      // Audit list — Peer → Task → Audits (mirrors amnesia: Peer → Session → Violations)
      const listEl = document.getElementById('watcher-list');
      if (!audits || !audits.length) {
        listEl.innerHTML = '<div class="empty-state">No audits yet &mdash; audits are created automatically when tasks complete</div>';
        return;
      }

      function gateIcon(passed) {
        return passed ? '<span style="color:var(--green)">&#x2713;</span>' : '<span style="color:var(--red)">&#x2717;</span>';
      }

      // Group: peer → task → audits
      const peerMap = {};
      const peerOrder = [];
      for (const a of audits) {
        const pid = a.source_node || 'unknown';
        const tid = a.task_id || 'unknown';
        if (!peerMap[pid]) {
          peerMap[pid] = { tasks: {}, taskOrder: [], count: 0 };
          peerOrder.push(pid);
        }
        const peer = peerMap[pid];
        if (!peer.tasks[tid]) {
          peer.tasks[tid] = { title: a.task_title, marker: a.task_marker, audits: [] };
          peer.taskOrder.push(tid);
        }
        peer.tasks[tid].audits.push(a);
        peer.count++;
      }

      let html = '';
      for (let pi = 0; pi < peerOrder.length; pi++) {
        const pid = peerOrder[pi];
        const pg = peerMap[pid];

        // Peer header (L1) — same as amnesia
        html += `<div class="tree-node peer-header clickable" data-wt-peer="${pi}">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pid)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-wt-peer-children="${pi}">`;

        // Task headers (L2) — same as amnesia sessions
        for (let ti = 0; ti < pg.taskOrder.length; ti++) {
          const tid = pg.taskOrder[ti];
          const tg = pg.tasks[tid];
          const latestStatus = tg.audits[0].status;
          const dotColor = latestStatus === 'passed' ? 'green' : latestStatus === 'failed' ? 'red' : 'yellow';

          html += `<div class="tree-node clickable" style="padding-left:24px" data-wt-task="${pi}-${ti}">
            <span class="tree-arrow">&#x25B6;</span>
            <span class="status-dot ${dotColor}"></span>
            <span>${esc(tg.title)}</span>
            <span class="session-uuid">${esc(tg.marker)}</span>
            <span class="count">${tg.audits.length}</span>
          </div>`;
          html += `<div style="display:none" data-wt-task-children="${pi}-${ti}">`;

          // Audit items (L3) — same as amnesia violations
          for (const a of tg.audits) {
            const statusClass = a.status === 'passed' ? 'confirmed' : a.status === 'failed' ? 'flagged' : 'dismissed';
            const downgraded = a.original_status !== a.final_status;

            html += `<div class="amnesia-item ${statusClass} clickable" data-id="${esc(a.id)}" style="margin-left:24px;cursor:pointer">
              <div class="amnesia-header">
                <span class="amnesia-status-label">${esc(a.status)}</span>
                <span class="badge type">Git ${gateIcon(a.git_passed)}</span>
                <span class="badge type">Build ${gateIcon(a.build_passed)}</span>
                <span class="badge type">Manifest ${gateIcon(a.manifest_passed)} ${a.manifest_score > 0 ? Math.round(a.manifest_score * 100) + '%' : ''}</span>
                ${downgraded ? '<span class="badge type" style="color:var(--red);font-weight:600">DOWNGRADED</span>' : ''}
                <span style="color:var(--text-muted);font-size:11px;margin-left:auto">${formatTime(a.audited_at)}</span>
              </div>
              <div class="amnesia-rule">
                <span style="color:var(--text-muted)">Manifest:</span> ${esc(a.manifest_title || 'none')}
              </div>
              <div class="amnesia-action">
                <span style="color:var(--text-muted)">Actions:</span> ${a.action_count}
                ${a.cost_usd > 0 ? `<span style="margin-left:12px;color:var(--text-muted)">Cost:</span> $${a.cost_usd.toFixed(2)}` : ''}
              </div>
            </div>`;
          }
          html += `</div>`;
        }
        html += `</div>`;
      }
      listEl.innerHTML = html;

      // Peer toggle (L1)
      listEl.querySelectorAll('[data-wt-peer]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.wtPeer;
          const children = listEl.querySelector(`[data-wt-peer-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Task toggle (L2)
      listEl.querySelectorAll('[data-wt-task]').forEach(node => {
        node.addEventListener('click', () => {
          const idx = node.dataset.wtTask;
          const children = listEl.querySelector(`[data-wt-task-children="${idx}"]`);
          const arrow = node.querySelector('.tree-arrow');
          if (children.style.display === 'none') {
            children.style.display = '';
            arrow.innerHTML = '&#x25BC;';
          } else {
            children.style.display = 'none';
            arrow.innerHTML = '&#x25B6;';
          }
        });
      });

      // Click handlers for detail view (L3)
      listEl.querySelectorAll('.amnesia-item.clickable').forEach(item => {
        item.addEventListener('click', () => loadWatcherDetail(item.dataset.id));
      });

    } catch(e) {
      console.error('Load watcher failed:', e);
    }
  }

  async function loadWatcherDetail(id) {
    try {
      const a = await fetchJSON('/api/watcher/audits/' + id);
      if (!a) return;

      const titleEl = document.getElementById('watcher-detail-title');
      const bodyEl = document.getElementById('watcher-detail');

      const statusColor = a.status === 'passed' ? 'var(--green)' : a.status === 'failed' ? 'var(--red)' : 'var(--yellow)';
      const statusLabel = a.status.toUpperCase();

      // Git gate detail
      const git = a.git_details || {};
      const gitColor = a.git_passed ? 'var(--green)' : 'var(--red)';
      const gitHtml = `
        <div style="border-left:3px solid ${gitColor};padding:8px 12px;margin-bottom:12px;background:var(--bg-secondary);border-radius:0 6px 6px 0">
          <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
            <span style="font-size:16px;color:${gitColor}">${a.git_passed ? '&#x2713;' : '&#x2717;'}</span>
            <span style="font-size:13px;font-weight:600">Gate 1: Git Verification</span>
          </div>
          <div style="font-size:12px;color:var(--text-secondary);margin-bottom:4px">${esc(git.reason || '')}</div>
          <div style="display:flex;gap:16px;font-size:11px;color:var(--text-muted)">
            <span>Commits: <strong>${git.commit_count || 0}</strong></span>
            <span>Files: <strong>${git.files_changed || 0}</strong></span>
            <span>+${git.insertions || 0} / -${git.deletions || 0}</span>
            ${git.uncommitted_files && git.uncommitted_files.length ? `<span style="color:var(--yellow)">Uncommitted: ${git.uncommitted_files.length}</span>` : ''}
          </div>
          ${git.uncommitted_files && git.uncommitted_files.length ? `
            <details style="margin-top:6px;font-size:11px">
              <summary style="cursor:pointer;color:var(--text-muted)">Uncommitted files (${git.uncommitted_files.length})</summary>
              <div style="margin-top:4px;font-family:var(--font-mono);font-size:10px;color:var(--text-secondary)">
                ${git.uncommitted_files.map(f => esc(f)).join('<br>')}
              </div>
            </details>
          ` : ''}
        </div>`;

      // Build gate detail
      const build = a.build_details || {};
      const buildColor = a.build_passed ? 'var(--green)' : 'var(--red)';
      const buildHtml = `
        <div style="border-left:3px solid ${buildColor};padding:8px 12px;margin-bottom:12px;background:var(--bg-secondary);border-radius:0 6px 6px 0">
          <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
            <span style="font-size:16px;color:${buildColor}">${a.build_passed ? '&#x2713;' : '&#x2717;'}</span>
            <span style="font-size:13px;font-weight:600">Gate 2: Build Verification</span>
          </div>
          <div style="font-size:12px;color:var(--text-secondary)">${esc(build.reason || '')}</div>
          ${build.output && !a.build_passed ? `
            <details style="margin-top:6px;font-size:11px">
              <summary style="cursor:pointer;color:var(--text-muted)">Build output</summary>
              <pre class="action-response" style="margin-top:4px;max-height:200px;overflow-y:auto;font-size:10px">${esc(build.output)}</pre>
            </details>
          ` : ''}
        </div>`;

      // Manifest gate detail
      const manifest = a.manifest_details || {};
      const manifestColor = a.manifest_passed ? 'var(--green)' : a.manifest_score >= 0.3 ? 'var(--yellow)' : 'var(--red)';
      const scorePercent = Math.round((a.manifest_score || 0) * 100);
      let deliverablesHtml = '';
      if (manifest.deliverables && manifest.deliverables.length) {
        deliverablesHtml = manifest.deliverables.map(d => {
          const dColor = d.status === 'done' ? 'var(--green)' : d.status === 'partial' ? 'var(--yellow)' : 'var(--red)';
          const dIcon = d.status === 'done' ? '&#x2713;' : d.status === 'partial' ? '&#x25CB;' : '&#x2717;';
          return `<div style="display:flex;align-items:flex-start;gap:8px;padding:3px 0;font-size:11px">
            <span style="color:${dColor};min-width:14px">${dIcon}</span>
            <span style="color:var(--text-secondary);flex:1">${esc(d.item)}</span>
            ${d.evidence ? `<span style="color:var(--text-muted);font-size:10px">${esc(d.evidence)}</span>` : ''}
          </div>`;
        }).join('');
      }

      const manifestHtml = `
        <div style="border-left:3px solid ${manifestColor};padding:8px 12px;margin-bottom:12px;background:var(--bg-secondary);border-radius:0 6px 6px 0">
          <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
            <span style="font-size:16px;color:${manifestColor}">${a.manifest_passed ? '&#x2713;' : '&#x2717;'}</span>
            <span style="font-size:13px;font-weight:600">Gate 3: Manifest Compliance</span>
            <span style="font-size:12px;font-weight:700;color:${manifestColor}">${scorePercent}%</span>
            <span style="font-size:11px;color:var(--text-muted)">(${manifest.done_items || 0}/${manifest.total_items || 0} deliverables)</span>
          </div>
          <div style="font-size:12px;color:var(--text-secondary);margin-bottom:4px">${esc(manifest.reason || '')}</div>
          ${deliverablesHtml ? `
            <div style="margin-top:8px;border-top:1px solid var(--border);padding-top:8px">
              <div style="font-size:11px;color:var(--text-muted);margin-bottom:4px;font-weight:600">Deliverables Checklist</div>
              ${deliverablesHtml}
            </div>
          ` : ''}
        </div>`;

      // Status verdict
      const downgraded = a.original_status !== a.final_status;
      const verdictHtml = downgraded ? `
        <div style="background:rgba(255,85,85,0.1);border:1px solid var(--red);border-radius:6px;padding:10px 14px;margin-bottom:12px">
          <span style="color:var(--red);font-weight:700;font-size:13px">&#x26A0; STATUS DOWNGRADED</span>
          <span style="font-size:12px;color:var(--text-secondary);margin-left:8px">
            Agent claimed <strong>${esc(a.original_status)}</strong> &rarr; Watcher set <strong>${esc(a.final_status)}</strong>
          </span>
        </div>` : '';

      titleEl.innerHTML = `<span style="color:${statusColor};font-weight:700">${statusLabel}</span> &mdash; ${esc(a.task_title)}`;

      bodyEl.innerHTML = `
        <div>
          <!-- BREADCRUMB -->
          <div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;font-family:var(--font-mono)">
            <span style="cursor:pointer;color:var(--accent)" onclick="switchView('watcher')">${esc(a.source_node ? a.source_node.substring(0,12) : 'node')}</span>
            <span style="opacity:0.4"> &rarr; </span>
            <span style="cursor:pointer;color:var(--accent)" onclick="switchView('tasks');setTimeout(()=>loadTaskDetail('${esc(a.task_id)}'),300)">${esc(a.task_marker)} ${esc(a.task_title)}</span>
            ${a.manifest_id ? `<span style="opacity:0.4"> &rarr; </span><span style="cursor:pointer;color:var(--accent)" onclick="switchView('manifests');setTimeout(()=>window._loadManifest('${esc(a.manifest_id)}'),300)">${esc(a.manifest_title || a.manifest_id.substring(0,12))}</span>` : ''}
            <span style="opacity:0.4"> &rarr; </span>
            <span style="color:var(--text-primary)">Audit</span>
          </div>
          <!-- Header -->
          <div style="display:flex;gap:16px;margin-bottom:12px;align-items:center;flex-wrap:wrap;font-size:12px">
            <div>
              <span style="color:var(--text-muted)">Task:</span>
              <span class="session-uuid" style="cursor:pointer;color:var(--accent);text-decoration:underline" onclick="switchView('tasks');setTimeout(()=>loadTaskDetail('${esc(a.task_marker)}'),300)">${esc(a.task_marker)}</span>
            </div>
            <div>
              <span style="color:var(--text-muted)">Manifest:</span>
              ${a.manifest_id ? `<span style="cursor:pointer;color:var(--accent);text-decoration:underline" onclick="switchView('manifests');setTimeout(()=>window._loadManifest('${esc(a.manifest_id)}'),300)">${esc(a.manifest_title || a.manifest_id.substring(0,12))}</span>` : '<span style="opacity:0.5">standalone</span>'}
            </div>
            <div>
              <span style="color:var(--text-muted)">Actions:</span>
              <strong>${a.action_count}</strong>
            </div>
            <div>
              <span style="color:var(--text-muted)">Cost:</span>
              <strong style="color:var(--green)">${a.cost_usd ? '$' + a.cost_usd.toFixed(2) : '$0.00'}</strong>
            </div>
            <div>
              <span style="color:var(--text-muted)">Audited:</span>
              ${new Date(a.audited_at).toLocaleString()}
            </div>
          </div>

          ${verdictHtml}
          ${gitHtml}
          ${buildHtml}
          ${manifestHtml}
        </div>`;

      // Highlight active item
      document.querySelectorAll('.watcher-item').forEach(i => i.style.background = '');
      const activeItem = document.querySelector(`.watcher-item[data-id="${a.id}"]`);
      if (activeItem) activeItem.style.background = 'var(--bg-secondary)';

    } catch(e) {
      console.error('Load watcher detail failed:', e);
    }
  }

  // Update watcher badge on overview load
  async function updateWatcherBadge() {
    try {
      const stats = await fetchJSON('/api/watcher/stats');
      const badge = document.getElementById('watcher-badge');
      if (stats && stats.failed > 0) {
        badge.textContent = stats.failed;
        badge.style.display = 'inline-block';
        badge.style.background = 'var(--red)';
        badge.style.color = '#fff';
        badge.style.borderRadius = '10px';
        badge.style.padding = '1px 6px';
        badge.style.fontSize = '10px';
        badge.style.fontWeight = '700';
        badge.style.marginLeft = '4px';
      } else {
        badge.style.display = 'none';
      }
    } catch(e) {}
  }

  // Call badge update on init
  updateWatcherBadge();

  // Listen for watcher events via WebSocket
  if (window._wsListeners) {
    window._wsListeners.push(function(event) {
      if (event.type === 'watcher_audit') {
        updateWatcherBadge();
        if (currentView === 'watcher') loadWatcher();
      }
    });
  }
})();
