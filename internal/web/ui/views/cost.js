(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatDuration = OL.formatDuration;

  var _costPeriod = 'day';
  var _costAgent = '';
  var _costThreshold = 0; // 0 = use server default
  var _lastCostData = null; // for CSV export
  var _lastDrillData = null; // for CSV export in drill-down

  // Agent color palette for drill-down grouping
  var _agentColors = ['var(--accent)', 'var(--green)', 'var(--yellow)', '#e879f9', '#fb923c', '#38bdf8', '#f87171', '#a78bfa'];
  function agentColor(agent, agents) {
    var idx = agents.indexOf(agent);
    return _agentColors[idx >= 0 ? idx % _agentColors.length : 0];
  }

  function getSelectedAgent() {
    var el = document.getElementById('cost-agent-select');
    return el ? el.value : '';
  }

  function getEffectiveBudget(serverBudget) {
    return _costThreshold > 0 ? _costThreshold : (serverBudget || 0);
  }

  // Load agent dropdown options
  async function loadCostAgents() {
    try {
      var agents = await fetchJSON('/api/tasks/cost-agents');
      var sel = document.getElementById('cost-agent-select');
      if (!sel || !agents) return;
      var current = sel.value;
      sel.innerHTML = '<option value="">All Agents</option>';
      for (var i = 0; i < agents.length; i++) {
        var a = agents[i];
        sel.innerHTML += '<option value="' + esc(a) + '"' + (a === current ? ' selected' : '') + '>' + esc(a) + '</option>';
      }
    } catch(e) {}
  }

  // Load trend summary cards
  async function loadCostTrend() {
    try {
      var agent = getSelectedAgent();
      var agentParam = agent ? '&agent=' + encodeURIComponent(agent) : '';
      var trend = await fetchJSON('/api/tasks/cost-trend?' + agentParam);
      if (!trend) return;
      var budget = getEffectiveBudget(100);
      var todayEl = document.getElementById('trend-today');
      var weekEl = document.getElementById('trend-week');
      var monthEl = document.getElementById('trend-month');
      var avgEl = document.getElementById('trend-avg');
      if (todayEl) {
        todayEl.textContent = '$' + trend.today.toFixed(2);
        todayEl.style.color = (budget > 0 && trend.today >= budget) ? 'var(--red)' : (budget > 0 && trend.today >= budget * 0.8) ? 'var(--yellow)' : 'var(--green)';
      }
      if (weekEl) weekEl.textContent = '$' + trend.this_week.toFixed(2);
      if (monthEl) monthEl.textContent = '$' + trend.this_month.toFixed(2);
      if (avgEl) avgEl.textContent = '$' + trend.avg_30d.toFixed(2);
    } catch(e) {}
  }

  OL.loadCostHistory = async function() {
    var tableEl = document.getElementById('cost-history-table');
    var chartEl = document.getElementById('cost-chart');
    var banner = document.getElementById('cost-drilldown-banner');
    if (!tableEl) return;

    loadCostAgents();
    loadCostTrend();

    var sel = document.getElementById('cost-days-select');
    var days = sel ? parseInt(sel.value) || 30 : 30;
    var agent = getSelectedAgent();
    var agentParam = agent ? '&agent=' + encodeURIComponent(agent) : '';

    try {
      var data = await fetchJSON('/api/tasks/cost-history?days=' + days + '&period=' + _costPeriod + agentParam);
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

      var serverBudget = data[0].budget || 0;
      var budget = getEffectiveBudget(serverBudget);
      var periodLabel = _costPeriod === 'week' ? 'Week' : _costPeriod === 'month' ? 'Month' : 'Date';

      // Chart
      if (chartEl) {
        var reversed = data.slice().reverse();
        var maxCost = Math.max.apply(null, reversed.map(function(d) { return d.cost; }).concat([budget || 1, 1]));
        var barWidth = Math.max(4, Math.floor((chartEl.clientWidth - 40) / reversed.length) - 2);
        var chartHtml = '<div style="display:flex;align-items:flex-end;gap:2px;height:100px;padding:8px 16px;position:relative">';
        if (budget > 0 && _costPeriod === 'day') {
          var budgetPx = Math.round((budget / maxCost) * 100);
          chartHtml += '<div style="position:absolute;left:16px;right:16px;bottom:' + (budgetPx + 8) + 'px;border-top:2px dashed var(--red);opacity:0.5"></div>';
          chartHtml += '<div style="position:absolute;right:20px;bottom:' + (budgetPx + 10) + 'px;font-size:9px;color:var(--red);opacity:0.7;font-weight:600">$' + budget.toFixed(0) + ' budget</div>';
        }
        for (var i = 0; i < reversed.length; i++) {
          var d = reversed[i];
          var h = maxCost > 0 ? Math.max(2, Math.round((d.cost / maxCost) * 100)) : 2;
          var overBudget = budget > 0 && d.cost >= budget;
          var color = overBudget ? 'var(--red)' : (budget > 0 && d.cost >= budget * 0.8) ? 'var(--yellow)' : 'var(--green)';
          chartHtml += '<div title="' + d.period + ': $' + d.cost.toFixed(2) + '" style="width:' + barWidth + 'px;height:' + h + 'px;background:' + color + ';border-radius:2px 2px 0 0;flex-shrink:0;opacity:' + (d.cost === 0 ? '0.2' : '0.8') + ';cursor:' + (_costPeriod === 'day' ? 'pointer' : 'default') + '" ' + (_costPeriod === 'day' ? 'onclick="loadCostDrillDown(\'' + d.period + '\')"' : '') + '></div>';
        }
        chartHtml += '</div>';
        chartEl.innerHTML = chartHtml;
      }

      // Table header
      var headerEl = document.getElementById('cost-table-header');
      if (headerEl) headerEl.textContent = _costPeriod === 'week' ? 'Weekly Breakdown' : _costPeriod === 'month' ? 'Monthly Breakdown' : 'Daily Breakdown';

      var html = '<table class="top-tasks-table" style="width:100%">' +
        '<thead><tr>' +
        '<th style="text-align:left;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">' + periodLabel + '</th>' +
        '<th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">tasks</th>' +
        '<th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">runs</th>' +
        '<th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">turns</th>' +
        '<th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">cost</th>' +
        '<th style="text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500">status</th>' +
        '</tr></thead><tbody>';

      var totalCost = 0, totalTurns = 0, totalTasks = 0, totalRuns = 0;
      for (var i = 0; i < data.length; i++) {
        var d = data[i];
        var overBudget = budget > 0 && d.cost >= budget && _costPeriod === 'day';
        var nearBudget = budget > 0 && d.cost >= budget * 0.8 && _costPeriod === 'day';
        var costColor = overBudget ? 'var(--red)' : nearBudget ? 'var(--yellow)' : 'var(--green)';
        var statusText = (_costPeriod === 'day' && budget > 0) ? (overBudget ? 'OVER' : 'OK') : '-';
        var statusColor = overBudget ? 'var(--red)' : 'var(--green)';
        var isToday = d.period === new Date().toISOString().split('T')[0];
        var rowBg = isToday ? 'background:rgba(0,217,126,0.04)' : '';
        var clickable = _costPeriod === 'day' && d.cost > 0;
        totalCost += d.cost;
        totalTurns += d.turns;
        totalTasks += d.tasks;
        totalRuns += (d.runs || 0);
        html += '<tr class="top-task-row" style="' + rowBg + ';' + (clickable ? 'cursor:pointer' : '') + '" ' + (clickable ? 'onclick="loadCostDrillDown(\'' + d.period + '\')"' : '') + '>' +
          '<td style="padding:6px 12px;font-family:var(--font-mono);font-size:12px;color:var(--accent)">' + d.period + (isToday ? ' <span style="font-size:10px;color:var(--green)">(today)</span>' : '') + '</td>' +
          '<td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">' + d.tasks + '</td>' +
          '<td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">' + (d.runs || d.tasks) + '</td>' +
          '<td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">' + d.turns + '</td>' +
          '<td style="padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;color:' + costColor + '">$' + d.cost.toFixed(2) + '</td>' +
          '<td style="padding:6px 12px;text-align:right;font-size:11px;font-weight:600;color:' + statusColor + '">' + statusText + '</td>' +
          '</tr>';
      }

      var avgCost = data.length > 0 ? totalCost / data.length : 0;
      var avgLabel = _costPeriod === 'week' ? '/week' : _costPeriod === 'month' ? '/month' : '/day';
      html += '<tr style="border-top:2px solid var(--border)">' +
        '<td style="padding:8px 12px;font-size:12px;font-weight:600;color:var(--text-secondary)">Total / Avg</td>' +
        '<td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600">' + totalTasks + '</td>' +
        '<td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600">' + (totalRuns || totalTasks) + '</td>' +
        '<td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600">' + totalTurns + '</td>' +
        '<td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600;color:var(--green)">$' + totalCost.toFixed(2) + '</td>' +
        '<td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:11px;color:var(--text-muted)">avg $' + avgCost.toFixed(2) + avgLabel + '</td>' +
        '</tr>';
      html += '</tbody></table>';
      tableEl.innerHTML = html;

    } catch (e) {
      tableEl.innerHTML = '<div class="empty-state">Failed to load cost history</div>';
    }
  };

  // Drill-down: show individual tasks for a specific date, grouped by agent
  window.loadCostDrillDown = async function(date) {
    var tableEl = document.getElementById('cost-history-table');
    var chartEl = document.getElementById('cost-chart');
    var banner = document.getElementById('cost-drilldown-banner');
    var bannerLabel = document.getElementById('cost-drilldown-label');
    if (!tableEl) return;

    var agent = getSelectedAgent();
    var agentParam = agent ? '&agent=' + encodeURIComponent(agent) : '';

    try {
      var data = await fetchJSON('/api/tasks/cost-history?date=' + date + agentParam);
      if (!data || !data.entries || !data.entries.length) {
        tableEl.innerHTML = '<div class="empty-state">No tasks found for ' + date + '</div>';
        _lastDrillData = null;
        return;
      }
      _lastDrillData = { date: date, entries: data.entries };
      _lastCostData = null;

      // Show drilldown banner
      if (banner && bannerLabel) {
        banner.style.display = 'flex';
        bannerLabel.innerHTML = 'Showing tasks for <strong style="color:var(--accent)">' + date + '</strong> (' + data.entries.length + ' runs, $' + data.entries.reduce(function(s, e) { return s + e.cost_usd; }, 0).toFixed(2) + ' total)';
      }

      // Hide chart in drilldown
      if (chartEl) chartEl.innerHTML = '';
      var headerEl = document.getElementById('cost-chart-header');
      if (headerEl) headerEl.textContent = 'Tasks on ' + date;

      // Group entries by agent
      var agentGroups = {};
      for (var i = 0; i < data.entries.length; i++) {
        var e = data.entries[i];
        var a = e.agent || 'unknown';
        if (!agentGroups[a]) agentGroups[a] = [];
        agentGroups[a].push(e);
      }
      var agentNames = Object.keys(agentGroups).sort();

      var html = '';
      for (var g = 0; g < agentNames.length; g++) {
        var agentName = agentNames[g];
        var entries = agentGroups[agentName];
        var groupCost = entries.reduce(function(s, e) { return s + e.cost_usd; }, 0);
        var groupTurns = entries.reduce(function(s, e) { return s + e.turns; }, 0);
        var color = agentColor(agentName, agentNames);

        // Agent group header
        html += '<div style="padding:8px 12px;background:var(--bg-secondary);border-bottom:1px solid var(--border);display:flex;align-items:center;gap:8px">' +
          '<span style="width:8px;height:8px;border-radius:50%;background:' + color + ';flex-shrink:0"></span>' +
          '<strong style="font-size:12px;color:' + color + '">' + esc(agentName) + '</strong>' +
          '<span style="font-size:11px;color:var(--text-muted);margin-left:auto">' + entries.length + ' runs</span>' +
          '<span style="font-size:11px;font-family:var(--font-mono);color:var(--text-secondary)">' + groupTurns + ' turns</span>' +
          '<span style="font-size:12px;font-family:var(--font-mono);font-weight:600;color:' + color + '">$' + groupCost.toFixed(2) + '</span>' +
          '</div>';

        html += '<table class="top-tasks-table" style="width:100%">' +
          '<thead><tr>' +
          '<th style="text-align:left;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">task</th>' +
          '<th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">run</th>' +
          '<th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">turns</th>' +
          '<th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">actions</th>' +
          '<th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">cost</th>' +
          '<th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">duration</th>' +
          '<th style="text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500">status</th>' +
          '</tr></thead><tbody>';

        for (var j = 0; j < entries.length; j++) {
          var e = entries[j];
          var statusColor = e.status === 'completed' ? 'var(--green)' : e.status === 'failed' ? 'var(--red)' : 'var(--yellow)';
          var dur = e.duration_sec > 0 ? formatDuration(e.duration_sec * 1000) : '-';
          var marker = e.task_marker || e.task_id.substring(0, 12);
          html += '<tr class="top-task-row" style="cursor:pointer" onclick="OL.switchView(\'tasks\');setTimeout(function(){OL.loadTaskDetail(\'' + esc(e.task_id) + '\')},300)">' +
            '<td style="padding:5px 12px;font-size:12px">' +
              '<span class="session-uuid" style="font-size:11px">' + esc(marker) + '</span>' +
              '<span style="color:var(--text-secondary);margin-left:4px">' + esc(e.task_title || 'Unknown') + '</span>' +
              (e.manifest_id ? '<span class="badge type" style="font-size:9px;margin-left:4px">' + esc(e.manifest_id.substring(0,12)) + '</span>' : '') +
            '</td>' +
            '<td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">#' + e.run_number + '</td>' +
            '<td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">' + e.turns + '</td>' +
            '<td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px">' + e.actions + '</td>' +
            '<td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;color:var(--green)">$' + e.cost_usd.toFixed(2) + '</td>' +
            '<td style="padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;color:var(--text-muted)">' + dur + '</td>' +
            '<td style="padding:5px 12px;text-align:right;font-size:11px;font-weight:600;color:' + statusColor + ';text-transform:uppercase">' + esc(e.status) + '</td>' +
            '</tr>';
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
    var csv = '';
    if (_lastDrillData && _lastDrillData.entries) {
      csv = 'date,task_id,task_marker,task_title,agent,run_number,status,actions,cost_usd,turns,duration_sec,started_at,completed_at\n';
      for (var i = 0; i < _lastDrillData.entries.length; i++) {
        var e = _lastDrillData.entries[i];
        csv += [_lastDrillData.date, e.task_id, e.task_marker, '"' + (e.task_title || '').replace(/"/g, '""') + '"', e.agent || '', e.run_number, e.status, e.actions, e.cost_usd.toFixed(4), e.turns, e.duration_sec || 0, e.started_at, e.completed_at].join(',') + '\n';
      }
    } else if (_lastCostData) {
      csv = 'period,tasks,runs,turns,cost,budget\n';
      for (var i = 0; i < _lastCostData.length; i++) {
        var d = _lastCostData[i];
        csv += [d.period, d.tasks, d.runs || d.tasks, d.turns, d.cost.toFixed(4), d.budget || 0].join(',') + '\n';
      }
    } else {
      return;
    }
    var blob = new Blob([csv], { type: 'text/csv' });
    var url = URL.createObjectURL(blob);
    var a = document.createElement('a');
    a.href = url;
    a.download = 'openloom-cost-' + new Date().toISOString().split('T')[0] + '.csv';
    a.click();
    URL.revokeObjectURL(url);
  }

  // Cost history controls
  document.getElementById('cost-back-btn')?.addEventListener('click', function() { OL.switchView('overview'); });
  document.getElementById('cost-days-select')?.addEventListener('change', function() { OL.loadCostHistory(); });
  document.getElementById('cost-drilldown-close')?.addEventListener('click', function() { OL.loadCostHistory(); });
  document.getElementById('cost-agent-select')?.addEventListener('change', function() { _costAgent = getSelectedAgent(); OL.loadCostHistory(); });
  document.getElementById('cost-export-csv')?.addEventListener('click', exportCostCSV);

  // Threshold selector
  (function initThreshold() {
    var saved = localStorage.getItem('openloom-cost-threshold');
    if (saved) {
      _costThreshold = parseFloat(saved) || 0;
      var sel = document.getElementById('cost-threshold-select');
      if (sel) {
        var match = Array.from(sel.options).find(function(o) { return o.value === String(_costThreshold); });
        if (match) sel.value = String(_costThreshold);
        else if (_costThreshold > 0) {
          sel.value = 'custom';
          var customEl = document.getElementById('cost-threshold-custom');
          if (customEl) { customEl.style.display = 'block'; customEl.value = _costThreshold; }
        }
      }
    }
  })();

  document.getElementById('cost-threshold-select')?.addEventListener('change', function() {
    var customEl = document.getElementById('cost-threshold-custom');
    if (this.value === 'custom') {
      if (customEl) customEl.style.display = 'block';
      return;
    }
    if (customEl) customEl.style.display = 'none';
    _costThreshold = parseFloat(this.value) || 0;
    localStorage.setItem('openloom-cost-threshold', String(_costThreshold));
    OL.loadCostHistory();
  });

  document.getElementById('cost-threshold-custom')?.addEventListener('change', function() {
    var val = parseFloat(this.value) || 0;
    if (val > 0) {
      _costThreshold = val;
      localStorage.setItem('openloom-cost-threshold', String(_costThreshold));
      OL.loadCostHistory();
    }
  });

  // Period tab clicks
  document.querySelectorAll('.cost-period-btn').forEach(function(btn) {
    btn.addEventListener('click', function() {
      _costPeriod = btn.dataset.period;
      document.querySelectorAll('.cost-period-btn').forEach(function(b) {
        b.style.background = 'transparent';
        b.style.color = 'var(--text-secondary)';
        b.classList.remove('active');
      });
      btn.style.background = 'var(--accent)';
      btn.style.color = 'var(--bg)';
      btn.classList.add('active');
      OL.loadCostHistory();
    });
  });
})(window.OL);
