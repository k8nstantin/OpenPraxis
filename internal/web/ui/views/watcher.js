(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  OL.loadWatcher = async function() {
    try {
      var results = await Promise.all([
        fetchJSON('/api/watcher/audits'),
        fetchJSON('/api/watcher/stats'),
      ]);
      var audits = results[0];
      var stats = results[1];

      // Stats bar
      var statsBar = document.getElementById('watcher-stats-bar');
      if (stats) {
        var passColor = stats.pass_rate >= 80 ? 'var(--green)' : stats.pass_rate >= 50 ? 'var(--yellow)' : 'var(--red)';
        statsBar.innerHTML =
          '<div class="metric-card">' +
            '<div class="metric-value">' + stats.total_audits + '</div>' +
            '<div class="metric-label">Total Audits</div>' +
          '</div>' +
          '<div class="metric-card">' +
            '<div class="metric-value" style="color:var(--green)">' + stats.passed + '</div>' +
            '<div class="metric-label">Passed</div>' +
          '</div>' +
          '<div class="metric-card">' +
            '<div class="metric-value" style="color:var(--red)">' + stats.failed + '</div>' +
            '<div class="metric-label">Failed</div>' +
          '</div>' +
          '<div class="metric-card">' +
            '<div class="metric-value" style="color:var(--yellow)">' + stats.warnings + '</div>' +
            '<div class="metric-label">Warnings</div>' +
          '</div>' +
          '<div class="metric-card">' +
            '<div class="metric-value" style="color:' + passColor + '">' + stats.pass_rate.toFixed(0) + '%</div>' +
            '<div class="metric-label">Pass Rate</div>' +
          '</div>';
      }

      // Audit list — Peer -> Task -> Audits
      var listEl = document.getElementById('watcher-list');

      function gateIcon(passed) {
        return passed ? '<span style="color:var(--green)">&#x2713;</span>' : '<span style="color:var(--red)">&#x2717;</span>';
      }

      // Group: peer -> task -> audits
      var peerMap = {};
      var peerOrder = [];
      for (var ai = 0; ai < (audits || []).length; ai++) {
        var a = audits[ai];
        var pid = a.source_node || 'unknown';
        var tid = a.task_id || 'unknown';
        if (!peerMap[pid]) {
          peerMap[pid] = { tasks: {}, taskOrder: [], count: 0 };
          peerOrder.push(pid);
        }
        var peer = peerMap[pid];
        if (!peer.tasks[tid]) {
          peer.tasks[tid] = { title: a.task_title, marker: a.task_marker, audits: [] };
          peer.taskOrder.push(tid);
        }
        peer.tasks[tid].audits.push(a);
        peer.count++;
      }

      // Transform into array for renderTree
      var treeData = peerOrder.map(function(pid) {
        var pg = peerMap[pid];
        return {
          peer_id: pid,
          count: pg.count,
          taskGroups: pg.taskOrder.map(function(tid) { return pg.tasks[tid]; }),
        };
      });

      OL.renderTree(listEl, treeData, {
        prefix: 'wt',
        emptyMessage: 'No audits yet &mdash; audits are created automatically when tasks complete',
        levels: [
          {
            label: function(pg) { return esc(pg.peer_id); },
            count: function(pg) { return pg.count; },
            children: function(pg) { return pg.taskGroups; },
          },
          {
            label: function(tg) { return esc(tg.title); },
            extra: function(tg) { return '<span class="session-uuid">' + esc(tg.marker) + '</span>'; },
            count: function(tg) { return tg.audits.length; },
            children: function(tg) { return tg.audits; },
            dotColor: function(tg) {
              var s = tg.audits[0].status;
              return s === 'passed' ? 'green' : s === 'failed' ? 'red' : 'yellow';
            },
            expanded: false,
          }
        ],
        renderLeaf: function(au) {
          var statusClass = au.status === 'passed' ? 'confirmed' : au.status === 'failed' ? 'flagged' : 'dismissed';
          var downgraded = au.original_status !== au.final_status;

          return '<div class="amnesia-item ' + statusClass + ' clickable tree-leaf" data-id="' + esc(au.id) + '" style="margin-left:24px;cursor:pointer">' +
            '<div class="amnesia-header">' +
              '<span class="amnesia-status-label">' + esc(au.status) + '</span>' +
              '<span class="badge type">Git ' + gateIcon(au.git_passed) + '</span>' +
              '<span class="badge type">Build ' + gateIcon(au.build_passed) + '</span>' +
              '<span class="badge type">Manifest ' + gateIcon(au.manifest_passed) + ' ' + (au.manifest_score > 0 ? Math.round(au.manifest_score * 100) + '%' : '') + '</span>' +
              (downgraded ? '<span class="badge type" style="color:var(--red);font-weight:600">DOWNGRADED</span>' : '') +
              '<span style="color:var(--text-muted);font-size:11px;margin-left:auto">' + formatTime(au.audited_at) + '</span>' +
            '</div>' +
            '<div class="amnesia-rule">' +
              '<span style="color:var(--text-muted)">Manifest:</span> ' + esc(au.manifest_title || 'none') +
            '</div>' +
            '<div class="amnesia-action">' +
              '<span style="color:var(--text-muted)">Actions:</span> ' + au.action_count +
              (au.cost_usd > 0 ? '<span style="margin-left:12px;color:var(--text-muted)">Cost:</span> $' + au.cost_usd.toFixed(2) : '') +
            '</div>' +
          '</div>';
        },
        leafSelector: '.tree-leaf',
        onLeafClick: function(el) { OL.loadWatcherDetail(el.dataset.id); },
      });

    } catch(e) {
      console.error('Load watcher failed:', e);
    }
  };

  OL.loadWatcherDetail = async function(id) {
    try {
      var a = await fetchJSON('/api/watcher/audits/' + id);
      if (!a) return;

      var titleEl = document.getElementById('watcher-detail-title');
      var bodyEl = document.getElementById('watcher-detail');

      var statusColor = a.status === 'passed' ? 'var(--green)' : a.status === 'failed' ? 'var(--red)' : 'var(--yellow)';
      var statusLabel = a.status.toUpperCase();

      // Git gate detail
      var git = a.git_details || {};
      var gitColor = a.git_passed ? 'var(--green)' : 'var(--red)';
      var gitHtml =
        '<div style="border-left:3px solid ' + gitColor + ';padding:8px 12px;margin-bottom:12px;background:var(--bg-secondary);border-radius:0 6px 6px 0">' +
          '<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">' +
            '<span style="font-size:16px;color:' + gitColor + '">' + (a.git_passed ? '&#x2713;' : '&#x2717;') + '</span>' +
            '<span style="font-size:13px;font-weight:600">Gate 1: Git Verification</span>' +
          '</div>' +
          '<div style="font-size:12px;color:var(--text-secondary);margin-bottom:4px">' + esc(git.reason || '') + '</div>' +
          '<div style="display:flex;gap:16px;font-size:11px;color:var(--text-muted)">' +
            '<span>Commits: <strong>' + (git.commit_count || 0) + '</strong></span>' +
            '<span>Files: <strong>' + (git.files_changed || 0) + '</strong></span>' +
            '<span>+' + (git.insertions || 0) + ' / -' + (git.deletions || 0) + '</span>' +
            (git.uncommitted_files && git.uncommitted_files.length ? '<span style="color:var(--yellow)">Uncommitted: ' + git.uncommitted_files.length + '</span>' : '') +
          '</div>' +
          (git.uncommitted_files && git.uncommitted_files.length ?
            '<details style="margin-top:6px;font-size:11px">' +
              '<summary style="cursor:pointer;color:var(--text-muted)">Uncommitted files (' + git.uncommitted_files.length + ')</summary>' +
              '<div style="margin-top:4px;font-family:var(--font-mono);font-size:10px;color:var(--text-secondary)">' +
                git.uncommitted_files.map(function(f) { return esc(f); }).join('<br>') +
              '</div>' +
            '</details>'
          : '') +
        '</div>';

      // Build gate detail
      var build = a.build_details || {};
      var buildColor = a.build_passed ? 'var(--green)' : 'var(--red)';
      var buildHtml =
        '<div style="border-left:3px solid ' + buildColor + ';padding:8px 12px;margin-bottom:12px;background:var(--bg-secondary);border-radius:0 6px 6px 0">' +
          '<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">' +
            '<span style="font-size:16px;color:' + buildColor + '">' + (a.build_passed ? '&#x2713;' : '&#x2717;') + '</span>' +
            '<span style="font-size:13px;font-weight:600">Gate 2: Build Verification</span>' +
          '</div>' +
          '<div style="font-size:12px;color:var(--text-secondary)">' + esc(build.reason || '') + '</div>' +
          (build.output && !a.build_passed ?
            '<details style="margin-top:6px;font-size:11px">' +
              '<summary style="cursor:pointer;color:var(--text-muted)">Build output</summary>' +
              '<pre class="action-response" style="margin-top:4px;max-height:200px;overflow-y:auto;font-size:10px">' + esc(build.output) + '</pre>' +
            '</details>'
          : '') +
        '</div>';

      // Manifest gate detail
      var manifest = a.manifest_details || {};
      var manifestColor = a.manifest_passed ? 'var(--green)' : a.manifest_score >= 0.3 ? 'var(--yellow)' : 'var(--red)';
      var scorePercent = Math.round((a.manifest_score || 0) * 100);
      var deliverablesHtml = '';
      if (manifest.deliverables && manifest.deliverables.length) {
        deliverablesHtml = manifest.deliverables.map(function(d) {
          var dColor = d.status === 'done' ? 'var(--green)' : d.status === 'partial' ? 'var(--yellow)' : 'var(--red)';
          var dIcon = d.status === 'done' ? '&#x2713;' : d.status === 'partial' ? '&#x25CB;' : '&#x2717;';
          return '<div style="display:flex;align-items:flex-start;gap:8px;padding:3px 0;font-size:11px">' +
            '<span style="color:' + dColor + ';min-width:14px">' + dIcon + '</span>' +
            '<span style="color:var(--text-secondary);flex:1">' + esc(d.item) + '</span>' +
            (d.evidence ? '<span style="color:var(--text-muted);font-size:10px">' + esc(d.evidence) + '</span>' : '') +
          '</div>';
        }).join('');
      }

      var manifestHtml =
        '<div style="border-left:3px solid ' + manifestColor + ';padding:8px 12px;margin-bottom:12px;background:var(--bg-secondary);border-radius:0 6px 6px 0">' +
          '<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">' +
            '<span style="font-size:16px;color:' + manifestColor + '">' + (a.manifest_passed ? '&#x2713;' : '&#x2717;') + '</span>' +
            '<span style="font-size:13px;font-weight:600">Gate 3: Manifest Compliance</span>' +
            '<span style="font-size:12px;font-weight:700;color:' + manifestColor + '">' + scorePercent + '%</span>' +
            '<span style="font-size:11px;color:var(--text-muted)">(' + (manifest.done_items || 0) + '/' + (manifest.total_items || 0) + ' deliverables)</span>' +
          '</div>' +
          '<div style="font-size:12px;color:var(--text-secondary);margin-bottom:4px">' + esc(manifest.reason || '') + '</div>' +
          (deliverablesHtml ?
            '<div style="margin-top:8px;border-top:1px solid var(--border);padding-top:8px">' +
              '<div style="font-size:11px;color:var(--text-muted);margin-bottom:4px;font-weight:600">Deliverables Checklist</div>' +
              deliverablesHtml +
            '</div>'
          : '') +
        '</div>';

      // Status verdict
      var downgraded = a.original_status !== a.final_status;
      var verdictHtml = downgraded ?
        '<div style="background:rgba(255,85,85,0.1);border:1px solid var(--red);border-radius:6px;padding:10px 14px;margin-bottom:12px">' +
          '<span style="color:var(--red);font-weight:700;font-size:13px">&#x26A0; STATUS DOWNGRADED</span>' +
          '<span style="font-size:12px;color:var(--text-secondary);margin-left:8px">' +
            'Agent claimed <strong>' + esc(a.original_status) + '</strong> &rarr; Watcher set <strong>' + esc(a.final_status) + '</strong>' +
          '</span>' +
        '</div>' : '';

      titleEl.innerHTML = '<span style="color:' + statusColor + ';font-weight:700">' + statusLabel + '</span> &mdash; ' + esc(a.task_title);

      var sourceLabel = esc(a.source_node ? a.source_node.substring(0,12) : 'node');
      bodyEl.innerHTML =
        '<div>' +
          '<!-- BREADCRUMB -->' +
          '<div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;font-family:var(--font-mono)">' +
            '<span style="cursor:pointer;color:var(--accent)" onclick="OL.switchView(\'watcher\')">' + sourceLabel + '</span>' +
            '<span style="opacity:0.4"> &rarr; </span>' +
            '<span style="cursor:pointer;color:var(--accent)" onclick="OL.switchView(\'tasks\');setTimeout(function(){OL.loadTaskDetail(\'' + esc(a.task_id) + '\')},300)">' + esc(a.task_marker) + ' ' + esc(a.task_title) + '</span>' +
            (a.manifest_id ?
              '<span style="opacity:0.4"> &rarr; </span>' +
              '<span style="cursor:pointer;color:var(--accent)" onclick="OL.switchView(\'manifests\');setTimeout(function(){OL.loadManifest(\'' + esc(a.manifest_id) + '\')},300)">' + esc(a.manifest_title || a.manifest_id.substring(0,12)) + '</span>'
            : '') +
            '<span style="opacity:0.4"> &rarr; </span>' +
            '<span style="color:var(--text-primary)">Audit</span>' +
          '</div>' +
          '<!-- Header -->' +
          '<div style="display:flex;gap:16px;margin-bottom:12px;align-items:center;flex-wrap:wrap;font-size:12px">' +
            '<div>' +
              '<span style="color:var(--text-muted)">Task:</span> ' +
              '<span class="session-uuid" style="cursor:pointer;color:var(--accent);text-decoration:underline" onclick="OL.switchView(\'tasks\');setTimeout(function(){OL.loadTaskDetail(\'' + esc(a.task_marker) + '\')},300)">' + esc(a.task_marker) + '</span>' +
            '</div>' +
            '<div>' +
              '<span style="color:var(--text-muted)">Manifest:</span> ' +
              (a.manifest_id ?
                '<span style="cursor:pointer;color:var(--accent);text-decoration:underline" onclick="OL.switchView(\'manifests\');setTimeout(function(){OL.loadManifest(\'' + esc(a.manifest_id) + '\')},300)">' + esc(a.manifest_title || a.manifest_id.substring(0,12)) + '</span>'
              : '<span style="opacity:0.5">standalone</span>') +
            '</div>' +
            '<div>' +
              '<span style="color:var(--text-muted)">Actions:</span> ' +
              '<strong>' + a.action_count + '</strong>' +
            '</div>' +
            '<div>' +
              '<span style="color:var(--text-muted)">Cost:</span> ' +
              '<strong style="color:var(--green)">' + (a.cost_usd ? '$' + a.cost_usd.toFixed(2) : '$0.00') + '</strong>' +
            '</div>' +
            '<div>' +
              '<span style="color:var(--text-muted)">Audited:</span> ' +
              new Date(a.audited_at).toLocaleString() +
            '</div>' +
          '</div>' +
          verdictHtml +
          gitHtml +
          buildHtml +
          manifestHtml +
        '</div>';

      // Highlight active item
      document.querySelectorAll('.watcher-item').forEach(function(i) { i.style.background = ''; });
      var activeItem = document.querySelector('.watcher-item[data-id="' + a.id + '"]');
      if (activeItem) activeItem.style.background = 'var(--bg-secondary)';

    } catch(e) {
      console.error('Load watcher detail failed:', e);
    }
  };

  OL.updateWatcherBadge = async function() {
    try {
      var stats = await fetchJSON('/api/watcher/stats');
      var badge = document.getElementById('watcher-badge');
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
  };

  // Call badge update on init
  OL.updateWatcherBadge();

  // Listen for watcher events via WebSocket
  if (OL._wsListeners) {
    OL._wsListeners.push(function(event) {
      if (event.type === 'watcher_audit') {
        OL.updateWatcherBadge();
        if (OL.currentView && OL.currentView() === 'watcher') OL.loadWatcher();
      }
    });
  }
})(window.OL);
