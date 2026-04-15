(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, setText = OL.setText, timeAgo = OL.timeAgo, formatTime = OL.formatTime;

  OL.updateMetrics = function(data) {
    setText('metric-memories', data.memories ?? '-');
    setText('metric-peers', data.peers ?? '-');
    setText('metric-agents', data.sessions ?? data.agents ?? '-');
    setText('metric-uptime', data.uptime || '-');
    setText('sidebar-peers', data.peers ?? 0);
    setText('sidebar-agents', data.sessions ?? data.agents ?? 0);
    var markerCount = data.markers ?? 0;
    setText('sidebar-markers', markerCount);
    var markerEl = document.getElementById('sidebar-markers');
    if (markerEl) markerEl.classList.toggle('has-markers', markerCount > 0);
  };

  OL.updateTaskStats = function(stats) {
    // Row 1: running, turns, cost, tasks
    var runningCount = stats.running ?? 0;
    setText('metric-running', runningCount);
    var runningCard = document.getElementById('metric-running-card');
    if (runningCard) {
      var valEl = runningCard.querySelector('.metric-value');
      if (valEl) valEl.style.color = runningCount > 0 ? 'var(--green)' : 'var(--text-muted)';
      runningCard.style.borderColor = runningCount > 0 ? 'var(--green)' : 'var(--border)';
    }

    setText('metric-turns-today', stats.turns_today ?? 0);
    var cost = stats.cost_today ?? 0;
    var budget = stats.daily_budget ?? 0;
    var budgetPct = stats.budget_pct ?? 0;

    // Show "$X / $Y" format when budget is set, otherwise just "$X"
    var costValEl = document.getElementById('metric-cost-today');
    if (budget > 0) {
      setText('metric-cost-today', '$' + cost.toFixed(2) + ' / $' + budget.toFixed(0));
      if (costValEl) costValEl.style.fontSize = '22px';
    } else {
      setText('metric-cost-today', '$' + cost.toFixed(2));
      if (costValEl) costValEl.style.fontSize = '';
    }

    // Color logic: green < 80%, yellow 80-100%, red > 100% (with pulse)
    var costEl = document.getElementById('metric-cost-today');
    var costCard = document.getElementById('metric-cost-card');
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
    var panel = document.getElementById('top-tasks-panel');
    var list = document.getElementById('top-tasks-list');
    var topTasks = stats.top_tasks || [];
    if (!topTasks.length) {
      if (panel) panel.style.display = 'none';
      return;
    }
    if (panel) panel.style.display = '';

    var statusColors = {running:'var(--green)',paused:'var(--yellow)',scheduled:'var(--yellow)',waiting:'var(--accent)',pending:'var(--text-muted)',completed:'var(--green)',max_turns:'var(--yellow)',failed:'var(--red)',cancelled:'var(--text-muted)'};

    var totalCost = topTasks.reduce(function(s, t) { return s + t.cost; }, 0);
    var html = '<div style="max-height:300px;overflow-y:auto"><table class="top-tasks-table" style="width:100%">' +
      '<thead style="position:sticky;top:0;background:var(--bg-primary);z-index:1"><tr>' +
        '<th style="text-align:left;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">marker</th>' +
        '<th style="text-align:left;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">title</th>' +
        '<th style="text-align:left;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">branch</th>' +
        '<th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">turns</th>' +
        '<th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">cost</th>' +
        '<th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">status</th>' +
      '</tr></thead><tbody>';

    for (var i = 0; i < topTasks.length; i++) {
      var t = topTasks[i];
      var costColor = t.cost > 5 ? 'var(--red)' : t.cost > 1 ? 'var(--yellow)' : 'var(--green)';
      var sColor = statusColors[t.status] || 'var(--text-muted)';
      var titleTrunc = t.title.length > 40 ? t.title.substring(0, 40) + '...' : t.title;
      html += '<tr class="top-task-row clickable" onclick="OL.switchView(\'tasks\');setTimeout(function(){OL.loadTaskDetail&&OL.loadTaskDetail(\'' + esc(t.marker) + '\')},200)">' +
        '<td style="padding:6px 12px;font-family:var(--font-mono);font-size:11px;color:var(--accent)">' + esc(t.marker) + '</td>' +
        '<td style="padding:6px 12px;font-size:12px">' + esc(titleTrunc) + '</td>' +
        '<td style="padding:6px 12px;font-family:var(--font-mono);font-size:10px;color:var(--text-muted)">openloom/' + esc(t.marker) + '</td>' +
        '<td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">' + t.turns + '</td>' +
        '<td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;color:' + costColor + '">$' + t.cost.toFixed(2) + '</td>' +
        '<td style="padding:6px 12px;text-align:right;font-size:11px;color:' + sColor + '">' + esc(t.status) + '</td>' +
      '</tr>';
    }
    html += '</tbody><tfoot><tr style="border-top:2px solid var(--border)">' +
      '<td colspan="4" style="padding:6px 12px;font-size:12px;font-weight:600">Total (' + topTasks.length + ' tasks)</td>' +
      '<td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600;color:var(--green)">$' + totalCost.toFixed(2) + '</td>' +
      '<td></td>' +
    '</tr></tfoot></table></div>';
    if (list) list.innerHTML = html;

    // Pending/scheduled tasks panel
    var pendingPanel = document.getElementById('pending-tasks-panel');
    var pendingList = document.getElementById('pending-tasks-list');
    var pending = stats.pending_tasks || [];
    if (!pending.length) {
      if (pendingPanel) pendingPanel.style.display = 'none';
    } else {
      if (pendingPanel) pendingPanel.style.display = '';
      var pendingStatusColors = {scheduled:'var(--yellow)',waiting:'var(--accent)',pending:'var(--text-muted)'};
      var statusIcons = {scheduled:'&#x23F0;',waiting:'&#x23F3;',pending:'&#x25CB;'};
      var phtml = '';
      for (var pi = 0; pi < pending.length; pi++) {
        var pt = pending[pi];
        var sc = pendingStatusColors[pt.status] || 'var(--text-muted)';
        var when = '';
        if (pt.next_run_at) {
          var diff = Math.round((new Date(pt.next_run_at) - Date.now()) / 60000);
          when = diff > 0 ? 'in ' + diff + 'm' : 'due';
        }
        if (pt.depends_on) when = 'after ' + pt.depends_on;
        phtml += '<div class="peer-row clickable" onclick="OL.switchView(\'tasks\')" style="padding:6px 0">' +
          '<span style="color:' + sc + ';font-size:12px">' + (statusIcons[pt.status] || '') + '</span>' +
          '<span class="session-uuid">' + esc(pt.marker) + '</span>' +
          '<span style="font-size:12px;flex:1">' + esc(pt.title.length > 40 ? pt.title.substring(0, 40) + '...' : pt.title) + '</span>' +
          '<span style="font-size:11px;color:' + sc + ';font-weight:500">' + when + '</span>' +
        '</div>';
      }
      if (pendingList) pendingList.innerHTML = phtml;
    }
  };

  OL.renderPeers = function(nodes) {
    var el = document.getElementById('overview-peers');
    if (!nodes.length) {
      el.innerHTML = '<div class="empty-state">No nodes</div>';
      return;
    }
    el.innerHTML = nodes.map(function(n) {
      var badge = n.is_local ? '<span class="badge scope">local</span>' : '<span class="badge tag">remote</span>';
      var sessions = (n.sessions || []).map(function(s) {
        return '<div class="agent-row" style="padding-left:20px">' +
          '<span class="status-dot green"></span>' +
          '<span class="agent-name">' + esc(s.agent) + '</span>' +
          '<span class="session-uuid">' + esc(s.uuid ? s.uuid.substring(0, 12) : '') + '</span>' +
          '<span class="agent-meta">' + (s.turn_count ? s.turn_count + ' turns' : (s.tool_calls || 0) + ' calls') + ' &middot; ' + timeAgo(s.connected_at) + '</span>' +
        '</div>';
      }).join('');
      var sessionCount = (n.sessions || []).length;
      return '<div class="node-group">' +
        '<div class="node-group-header">' +
          '<span class="status-dot ' + (n.status === 'online' ? 'green' : 'yellow') + '"></span>' +
          esc(n.node_id) + ' ' + badge +
          '<span style="margin-left:auto;font-size:11px;color:var(--text-muted)">' + (n.memories || 0) + ' memories &middot; ' + sessionCount + ' session' + (sessionCount !== 1 ? 's' : '') + '</span>' +
        '</div>' +
        (sessions || '<div style="padding:4px 20px;font-size:12px;color:var(--text-muted)">No active sessions</div>') +
      '</div>';
    }).join('');
  };

  OL.renderPeersList = function(nodes) {
    var el = document.getElementById('peers-list');
    if (!nodes.length) {
      el.innerHTML = '<div class="empty-state">No nodes</div>';
      return;
    }
    el.innerHTML = nodes.map(function(n) {
      var badge = n.is_local ? '<span class="badge scope">local</span>' : '<span class="badge tag">remote</span>';
      var sessions = (n.sessions || []).map(function(s) {
        return '<div class="agent-row" style="padding-left:20px">' +
          '<span class="status-dot green"></span>' +
          '<span class="agent-name">' + esc(s.agent) + '</span>' +
          '<span class="session-uuid">' + esc(s.uuid ? s.uuid.substring(0, 12) : '') + '</span>' +
          '<span class="agent-meta">' + (s.turn_count ? s.turn_count + ' turns' : (s.tool_calls || 0) + ' calls') + ' &middot; ' + timeAgo(s.connected_at) + '</span>' +
        '</div>';
      }).join('');
      var sessionCount = (n.sessions || []).length;
      return '<div class="node-group" style="margin-bottom:16px">' +
        '<div class="node-group-header" style="font-size:14px">' +
          '<span class="status-dot ' + (n.status === 'online' ? 'green' : 'yellow') + '"></span>' +
          esc(n.node_id) + ' ' + badge +
          '<span style="margin-left:auto;font-size:12px;color:var(--text-muted)">' + (n.memories || 0) + ' memories</span>' +
        '</div>' +
        (sessions || '<div style="padding:8px 20px;font-size:13px;color:var(--text-muted)">No active sessions</div>') +
      '</div>';
    }).join('');
  };

  OL.loadPeersView = async function() {
    try {
      var nodes = await fetchJSON('/api/peers');
      OL.renderPeersList(nodes || []);
    } catch (e) {
      console.error('Load peers failed:', e);
    }
  };

  OL.renderRecentMemories = function(mems) {
    var el = document.getElementById('overview-memories');
    if (!mems.length) {
      el.innerHTML = '<div class="empty-state">No memories stored yet</div>';
      return;
    }
    el.innerHTML = mems.slice(0, 10).map(function(m) {
      var marker = m.id ? m.id.substring(0, 12) : '';
      var session = m.source_agent || '';
      return '<div class="memory-row clickable" data-memory-id="' + esc(m.id) + '">' +
        '<span class="session-uuid">' + esc(marker) + '</span>' +
        '<span class="badge type">' + esc(m.type) + '</span>' +
        '<span style="color:var(--text-primary);font-size:13px">' + esc(m.l0) + '</span>' +
        '<span style="color:var(--text-muted);font-size:11px;margin-left:auto">' + esc(session) + '</span>' +
      '</div>';
    }).join('');
    el.querySelectorAll('.memory-row').forEach(function(row) {
      row.addEventListener('click', function() {
        OL.switchView('memories');
        OL.loadMemoryPeerDetail(row.dataset.memoryId);
      });
    });
  };

  OL.loadTaskStats = async function() {
    try {
      var stats = await fetchJSON('/api/tasks/stats');
      if (stats) OL.updateTaskStats(stats);
    } catch (e) {}
  };

  OL.loadRunningTasks = async function() {
    try {
      var tasks = await fetchJSON('/api/tasks/running');
      var hasRunning = tasks && tasks.length > 0;

      // Render to both overview and activity panels
      var targets = [
        {panel: 'running-tasks-panel', list: 'running-tasks-list', badge: 'running-tasks-count'},
        {panel: 'activity-running-panel', list: 'activity-running-list', badge: null},
        {panel: 'tasks-running-panel', list: 'tasks-running-list', badge: null},
      ];

      for (var ti = 0; ti < targets.length; ti++) {
        var t = targets[ti];
        var panel = document.getElementById(t.panel);
        var el = document.getElementById(t.list);
        if (!panel || !el) continue;

        if (!hasRunning) {
          panel.style.display = 'none';
          continue;
        }

        panel.style.display = '';
        if (t.badge) {
          var badge = document.getElementById(t.badge);
          if (badge) badge.textContent = tasks.length;
        }

        el.innerHTML = tasks.map(function(rt) {
          var elapsed = Math.round((Date.now() - new Date(rt.started_at).getTime()) / 1000);
          var mins = Math.floor(elapsed / 60);
          var secs = elapsed % 60;
          return '<div class="peer-row clickable running-task-row" data-task-id="' + esc(rt.task_id) + '" style="flex-wrap:wrap;cursor:pointer">' +
            '<span class="status-dot ' + (rt.paused ? 'yellow' : 'green') + '" style="' + (rt.paused ? '' : 'animation:pulse 1s infinite') + '"></span>' +
            '<span class="session-uuid">' + esc(rt.marker) + '</span>' +
            '<span style="font-weight:500;font-size:13px;flex:1">' + esc(rt.title) + '</span>' +
            '<span class="badge type" style="font-size:10px">' + esc(rt.agent) + '</span>' +
            '<span style="font-size:12px;color:var(--text-muted)">' + rt.actions + ' actions</span>' +
            '<span style="font-size:12px;color:' + (rt.paused ? 'var(--yellow)' : 'var(--green)') + ';font-weight:500">' + (rt.paused ? 'PAUSED' : mins + 'm ' + secs + 's') + '</span>' +
            (rt.paused
              ? '<button class="btn-search" onclick="event.stopPropagation();OL.resumeTask(\'' + esc(rt.task_id) + '\')" style="font-size:11px;padding:3px 10px">Resume</button>'
              : '<button class="btn-search" onclick="event.stopPropagation();OL.pauseTask(\'' + esc(rt.task_id) + '\')" style="font-size:11px;padding:3px 10px;background:var(--yellow);color:var(--bg-primary)">Pause</button>') +
            '<button class="btn-confirm" onclick="event.stopPropagation();OL.killTask(\'' + esc(rt.task_id) + '\')" style="font-size:11px;padding:3px 10px">Stop</button>' +
          '</div>';
        }).join('');

        // Click row -> go to Tasks tab -> open task detail with live output
        el.querySelectorAll('.running-task-row').forEach(function(row) {
          row.addEventListener('click', function() {
            OL.switchView('tasks');
            setTimeout(function() { OL.loadTaskDetail(row.dataset.taskId); }, 300);
          });
        });
      }
    } catch (e) {}
  };

  OL.killTask = async function(id) {
    if (!confirm('Stop this running task?')) return;
    await fetch('/api/tasks/' + id + '/kill', {method: 'POST'});
    OL.loadRunningTasks();
    OL.loadTaskDetail(id);
    OL.loadTasks();
  };

  OL.pauseTask = async function(id) {
    await fetch('/api/tasks/' + id + '/pause', {method: 'POST'});
    OL.loadRunningTasks();
    OL.loadTaskDetail(id);
    OL.loadTasks();
  };

  OL.resumeTask = async function(id) {
    await fetch('/api/tasks/' + id + '/resume', {method: 'POST'});
    OL.loadRunningTasks();
    OL.loadTaskDetail(id);
    OL.loadTasks();
  };

  OL.emergencyStopAll = async function() {
    if (!confirm('EMERGENCY STOP — Kill ALL running tasks?')) return;
    try {
      var tasks = await fetchJSON('/api/tasks/running');
      if (tasks) {
        for (var i = 0; i < tasks.length; i++) {
          await fetch('/api/tasks/' + tasks[i].task_id + '/kill', {method: 'POST'});
        }
      }
      OL.loadRunningTasks();
    } catch (e) {}
  };

  // Wire emergency stop buttons
  document.addEventListener('DOMContentLoaded', function() {
    var btn1 = document.getElementById('emergency-stop-btn');
    var btn2 = document.getElementById('activity-emergency-stop');
    if (btn1) btn1.onclick = OL.emergencyStopAll;
    if (btn2) btn2.onclick = OL.emergencyStopAll;
  });
})(window.OL);
