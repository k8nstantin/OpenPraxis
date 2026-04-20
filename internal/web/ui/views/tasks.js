(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime, formatDuration = OL.formatDuration;

  // --- Parse Task Output ---
  OL.parseTaskOutput = function(raw) {
    if (!raw) return '<div class="empty-state">No output</div>';
    var lines = raw.split('\n').filter(l => l.trim());
    var totalLines = lines.length;
    // For large outputs, only parse — keep all lines but cap display
    if (lines.length > 500) {
      lines = lines.slice(-500);
    }
    var html = '';
    var turnNum = 0;
    var resultInfo = null;

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
                <span class="badge type badge-sm">${esc(name)}</span>
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
  };

  // --- Tasks ---
  function renderTaskSearchList(el, tasks) {
    if (!tasks || !tasks.length) {
      el.innerHTML = '<div class="empty-state" style="padding:16px">No tasks found</div>';
      return;
    }
    el.innerHTML = tasks.map(function(t) {
      var render = OL.getStatusRender(t.status);
      var meta = [];
      if (t.run_count > 0) meta.push(t.run_count + ' runs');
      if (t.total_turns > 0) meta.push(t.total_turns + ' turns');
      if (t.total_cost > 0) meta.push('$' + t.total_cost.toFixed(2));
      if (t.schedule) meta.push(esc(t.schedule));
      return '<div class="amnesia-item ' + render.cssClass + ' clickable" data-id="' + esc(t.id) + '" ' +
        'role="button" tabindex="0" ' +
        'onclick="OL.loadTaskDetail(\'' + esc(t.id) + '\')" ' +
        'onkeydown="if(event.key===\'Enter\'||event.key===\' \'){event.preventDefault();this.click()}">' +
        '<div class="amnesia-header">' +
          '<span class="status-dot" style="background:' + render.color + '"></span>' +
          '<span class="amnesia-status-label" style="color:' + render.color + ';font-weight:600">' +
            render.icon + ' ' + esc(t.status) +
          '</span>' +
          '<span class="session-uuid">' + esc(t.marker) + '</span>' +
        '</div>' +
        '<div class="amnesia-rule">' + esc(t.title) + '</div>' +
        (meta.length ? '<div class="amnesia-action">' + meta.join(' &middot; ') + '</div>' : '') +
      '</div>';
    }).join('');
  }

  OL.loadTasks = async function() {
    const el = document.getElementById('tasks-list');
    const newBtn = document.getElementById('task-new-btn');

    newBtn.onclick = () => OL.showTaskCreateForm();

    const mount = document.getElementById('tasks-search-mount');
    if (mount && OL.mountSearchInput) {
      OL.mountSearchInput(mount, {
        placeholder: 'Search tasks by id, marker, title, or keyword...',
        onSearch: async function(q) {
          const results = await fetchJSON('/api/tasks/search?q=' + encodeURIComponent(q));
          renderTaskSearchList(el, results || []);
          return (results || []).length;
        },
        onClear: function() { OL.loadTasks(); }
      });
    }

    // Status styling comes from OL.getStatusRender (task-status.js).
    // Never add an inline map here — adding a status in status.go
    // should update exactly one place: task-status.js.

    try {
      var peerGroups = await fetchJSON('/api/tasks/by-peer');

      // Filter out empty manifests before passing to renderTree
      var treeData = (peerGroups || []).map(function(pg) {
        return {
          peer_id: pg.peer_id,
          manifests: pg.manifests.filter(function(mg) { return mg.tasks.length > 0; }),
          totalTasks: pg.manifests.reduce(function(sum, mg) { return sum + mg.tasks.length; }, 0),
        };
      });

      OL.renderTree(el, treeData, {
        prefix: 'task',
        emptyMessage: 'No tasks yet',
        levels: [
          {
            label: function(pg) { return esc(pg.peer_id); },
            count: function(pg) { return pg.totalTasks; },
            children: function(pg) { return pg.manifests; },
          },
          {
            label: function(mg) { return esc(mg.manifest_title || 'Standalone'); },
            extra: function(mg) { return '<span class="session-uuid">' + esc(mg.manifest_marker || '') + '</span>'; },
            count: function(mg) { return mg.tasks.length; },
            children: function(mg) { return mg.tasks; },
            dotColor: function(mg) {
              // Aggregate color for the manifest group: red if any
              // task is failed, green if any is running, yellow if any
              // is armed (scheduled or waiting), grey otherwise.
              var hasFailed = mg.tasks.some(function(t) { return t.status === 'failed'; });
              if (hasFailed) return 'red';
              var hasRunning = mg.tasks.some(function(t) { return t.status === 'running'; });
              if (hasRunning) return 'green';
              var hasArmed = mg.tasks.some(function(t) { return t.status === 'scheduled' || t.status === 'waiting' || t.status === 'paused'; });
              if (hasArmed) return 'yellow';
              return 'green';
            },
            expanded: false,
          }
        ],
        renderLeaf: function(t) {
          var render = OL.getStatusRender(t.status);
          var metaParts = [];
          if (t.run_count > 0) metaParts.push(t.run_count + ' runs');
          if (t.total_turns > 0) metaParts.push(t.total_turns + ' turns');
          if (t.total_cost > 0) metaParts.push('$' + t.total_cost.toFixed(2));
          metaParts.push(t.schedule);

          // block_reason surfaces the manifest/task that's holding this
          // task in waiting. Clickable in the detail view; here it's
          // a tooltip + short inline hint so the row stays compact.
          var blockHint = '';
          if (t.status === 'waiting' && t.block_reason) {
            blockHint = ' &middot; <span style="color:var(--accent)" title="' + esc(t.block_reason) + '">blocked</span>';
          }

          var pulseClass = render.pulse ? ' status-pulse' : '';

          return '<div class="amnesia-item ' + render.cssClass + ' clickable tree-leaf tree-indent" data-id="' + esc(t.id) + '">' +
            '<div class="amnesia-header">' +
              '<span class="status-dot" style="background:' + render.color + ';' + (render.pulse ? 'animation:pulse 1s infinite' : '') + '"></span>' +
              '<span class="amnesia-status-label" style="color:' + render.color + ';font-weight:600" title="' + esc(render.label) + '">' +
                render.icon + ' ' + esc(t.status) +
              '</span>' +
              '<span class="session-uuid">' + esc(t.marker) + '</span>' +
              '<span class="badge type">' + esc(t.schedule) + '</span>' +
              (t.depends_on ? '<span class="badge scope">dep</span>' : '') +
              '<span class="meta-time">' + formatTime(t.updated_at || t.created_at) + '</span>' +
            '</div>' +
            '<div class="amnesia-rule">' + esc(t.title) + '</div>' +
            '<div class="amnesia-action">' + metaParts.join(' &middot; ') + blockHint + '</div>' +
          '</div>';
        },
        leafSelector: '.tree-leaf',
        onLeafClick: function(item) {
          el.querySelectorAll('.amnesia-item').forEach(function(i) { i.classList.remove('active'); });
          item.classList.add('active');
          OL.loadTaskDetail(item.dataset.id);
        },
      });
    } catch (e) {
      console.error('Load tasks failed:', e);
    }
  };

  OL.loadTaskDetail = async function(id) {
    try {
      const t = await fetchJSON('/api/tasks/' + id);
      if (!t) return;
      const titleEl = document.getElementById('task-detail-title');
      const bodyEl = document.getElementById('task-detail');

      const statusRender = OL.getStatusRender(t.status);
      const statusColor = statusRender.color;

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

      // Fetch linked actions, amnesia, delusions, run history, and
      // execution_review comments in parallel. The exec-review payload drives
      // the "No execution review" badge (M4-T10) in the header.
      let actionsHtml = '', amnesiaHtml = '', delusionsHtml = '', runsHtml = '';
      let needsExecReview = false;
      try {
        const [actions, amnesia, delusions, runs, execReviewRes] = await Promise.all([
          fetchJSON('/api/tasks/' + t.id + '/actions'),
          fetchJSON('/api/tasks/' + t.id + '/amnesia'),
          fetchJSON('/api/tasks/' + t.id + '/delusions'),
          fetchJSON('/api/tasks/' + t.id + '/runs'),
          fetchJSON('/api/tasks/' + t.id + '/comments?type=execution_review'),
        ]);
        // Badge condition: task is completed AND no agent-authored
        // execution_review comment exists. execReviewRes shape:
        // { comments: [{ author, type, ... }, ...] }.
        if (t.status === 'completed') {
          const rows = (execReviewRes && execReviewRes.comments) || [];
          const hasAgent = rows.some(c => c && c.author === 'agent');
          needsExecReview = !hasAgent;
        }
        if (actions && actions.length) {
          actionsHtml = `<div style="margin-bottom:12px">
            <div class="section-label">Actions (${actions.length})</div>
            <div style="display:flex;flex-direction:column;gap:2px">
              ${actions.slice(0, 50).map((a, i) => {
                const inputPreview = a.tool_input ? (a.tool_input.length > 80 ? a.tool_input.substring(0, 80) + '...' : a.tool_input) : '';
                const hasResponse = a.tool_response && a.tool_response.length > 0;
                return `<div class="task-action-item" style="border:1px solid var(--border);border-radius:4px;padding:6px 8px;font-size:11px">
                  <div role="button" tabindex="0" aria-expanded="false" style="display:flex;align-items:center;gap:6px;cursor:pointer" onclick="var d=this.parentElement.querySelector('.task-action-detail');var v=d.style.display==='none';d.style.display=v?'':'none';this.setAttribute('aria-expanded',v)" onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();this.click()}">
                    <span style="color:var(--text-muted);font-size:10px;min-width:16px">#${actions.length - i}</span>
                    <span class="badge type badge-sm">${esc(a.tool_name)}</span>
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
              <span class="amnesia-score high badge-sm">${Math.round(a.score*100)}%</span>
              <span style="color:var(--text-secondary)">Rule [${esc(a.rule_marker)}]:</span> ${esc(a.rule_text.length > 60 ? a.rule_text.substring(0,60) + '...' : a.rule_text)}
            </div>`).join('')}
          </div>`;
        }
        if (delusions && delusions.length) {
          delusionsHtml = `<div style="margin-bottom:12px;border-left:3px solid var(--yellow);padding-left:10px">
            <div style="font-size:12px;color:var(--yellow);margin-bottom:6px;font-weight:500">Manifest Deviations (${delusions.length})</div>
            ${delusions.map(d => `<div style="font-size:12px;margin-bottom:4px">
              <span class="amnesia-score medium badge-sm">${Math.round(d.score*100)}%</span>
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
                return `<div class="task-run-item" role="button" tabindex="0" style="border:1px solid var(--border);border-radius:4px;padding:6px 8px;font-size:11px;cursor:pointer" data-run-id="${r.id}" data-task-id="${esc(t.id)}">
                  <div class="flex-row">
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
          <div class="breadcrumb">
            <span class="breadcrumb-link" onclick="OL.switchView('${taskProduct ? 'products' : 'tasks'}')">${esc(t.source_node ? t.source_node.substring(0,12) : 'node')}</span>
            ${taskProduct ? `<span class="breadcrumb-sep"> → </span><span class="breadcrumb-link" onclick="OL.switchView('products');setTimeout(()=>OL.loadProduct('${esc(taskProduct.id)}'),300)">${esc(taskProduct.marker)} ${esc(taskProduct.title)}</span>` : ''}
            ${t.manifest_id ? `<span class="breadcrumb-sep"> → </span><span class="breadcrumb-link" onclick="OL.switchView('manifests');setTimeout(()=>OL.loadManifest('${esc(t.manifest_id)}'),300)">${esc(taskManifest ? taskManifest.marker + ' ' + taskManifest.title : t.manifest_id.substring(0,12))}</span>` : ''}
            <span class="breadcrumb-sep"> → </span>
            <span style="color:var(--text-primary)">${esc(t.marker)} ${esc(t.title)}</span>
          </div>
          <!-- 1. HEADER BAR -->
          <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
            <span class="session-uuid" style="font-size:14px">${esc(t.marker)}</span>
            <span style="flex:1;font-size:15px;font-weight:600;color:var(--text-primary)">${esc(t.title)}</span>
            <span style="color:${statusColor};font-weight:700;font-size:13px;text-transform:uppercase;padding:3px 10px;border:2px solid ${statusColor};border-radius:4px" title="${esc(statusRender.label)}">${statusRender.icon} ${esc(t.status)}</span>
            ${needsExecReview ? `<a href="#task-comments-mount" onclick="var el=document.getElementById('task-comments-mount');if(el)el.scrollIntoView({behavior:'smooth'});return false;" style="color:var(--yellow);font-weight:700;font-size:11px;text-transform:uppercase;padding:3px 8px;border:2px solid var(--yellow);border-radius:4px;text-decoration:none;cursor:pointer" title="Task completed but no agent-authored execution_review comment was posted. Click to jump to the comments section.">&#9888; No execution review</a>` : ''}
          </div>
          ${t.block_reason ? `
          <div style="display:flex;align-items:center;gap:6px;margin-bottom:12px;padding:6px 10px;background:rgba(59,130,246,0.08);border-left:3px solid var(--accent);border-radius:4px;font-size:12px;color:var(--text-primary)">
            <span style="color:var(--accent);font-weight:600">Blocked:</span>
            <span>${esc(t.block_reason)}</span>
          </div>` : ''}

          <!-- METADATA — runs, turns, cost, agent, manifest, branch -->
          <div style="display:flex;gap:12px;font-size:12px;color:var(--text-muted);margin-bottom:16px;align-items:center;flex-wrap:wrap;padding:8px 12px;background:var(--bg-secondary);border:1px solid var(--border);border-radius:6px;font-family:var(--font-mono)">
            <span>Runs: <strong style="color:var(--text-primary)">${t.run_count}</strong></span>
            <span>Turns: <strong style="color:var(--text-primary)">${t.total_turns || 0}</strong></span>
            <span>Cost: <strong style="color:var(--green)">$${(t.total_cost || 0).toFixed(2)}</strong></span>
            <span class="separator">|</span>
            <span>Agent: <strong style="color:var(--text-primary)">${esc(t.agent)}</strong></span>
            <span>Branch: <strong style="color:var(--text-primary)">openpraxis/${esc(t.marker)}</strong></span>
            ${t.manifest_id ? `<span>Manifest: <span class="manifest-nav" style="cursor:pointer;color:var(--accent);text-decoration:underline;font-weight:600" data-mid="${esc(t.manifest_id)}">${esc(t.manifest_id.substring(0,12))} &#x2192;</span></span>` : '<span>standalone</span>'}
            ${t.last_run_at ? `<span class="separator">|</span><span>Last: ${new Date(t.last_run_at).toLocaleString()}</span>` : ''}
            <span>Created: ${new Date(t.created_at).toLocaleString()}</span>
          </div>

          <!-- 2. CONTROL BAR — ALWAYS VISIBLE -->
          <div style="margin-bottom:16px;padding:12px;border:1px solid var(--border);border-radius:8px;background:var(--bg-secondary)">
            <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:${isRunningOrPaused ? '0' : '12'}px">
              ${t.status === 'pending' || t.status === 'waiting' ? `
                <button class="btn-search" onclick="OL.taskStart('${esc(t.id)}')">&#9654; Start Now</button>
                <button class="btn-search" style="background:var(--bg-input)" onclick="OL.rescheduleTask('${esc(t.id)}')">&#128260; Reschedule</button>
                <button class="btn-dismiss" onclick="OL.taskArchive('${esc(t.id)}')">Archive</button>
              ` : ''}
              ${t.status === 'scheduled' ? `
                <button class="btn-search" onclick="OL.taskStart('${esc(t.id)}')">&#9654; Start Now</button>
                <button class="btn-dismiss" onclick="OL.taskAction('${esc(t.id)}','cancel')">&#10005; Cancel</button>
                <button class="btn-search" style="background:var(--bg-input)" onclick="OL.rescheduleTask('${esc(t.id)}')">&#128260; Reschedule</button>
                <button class="btn-dismiss" onclick="OL.taskArchive('${esc(t.id)}')">Archive</button>
              ` : ''}
              ${t.status === 'running' ? `
                <button class="btn-search" style="background:var(--yellow);color:var(--bg-primary)" onclick="OL.pauseTask('${esc(t.id)}')">&#9208; Pause</button>
                <button class="btn-confirm" onclick="OL.killTask('${esc(t.id)}')">&#9209; Stop</button>
              ` : ''}
              ${t.status === 'paused' ? `
                <button class="btn-search" onclick="OL.resumeTask('${esc(t.id)}')">&#9654; Resume</button>
                <button class="btn-search" style="background:var(--accent)" onclick="OL.editInstructions('${esc(t.id)}')">&#9998; Edit Instructions</button>
                <button class="btn-confirm" onclick="OL.killTask('${esc(t.id)}')">&#9209; Stop</button>
              ` : ''}
              ${t.status === 'completed' || t.status === 'failed' || t.status === 'cancelled' ? `
                <button class="btn-search" onclick="OL.taskStart('${esc(t.id)}')">&#128260; Restart</button>
                <button class="btn-search" style="background:var(--bg-input)" onclick="OL.rescheduleTask('${esc(t.id)}')">&#128260; Reschedule</button>
                <button class="btn-dismiss" onclick="OL.taskArchive('${esc(t.id)}')">Archive</button>
              ` : ''}
            </div>

            ${!isRunningOrPaused ? `
            <!-- 3. SCHEDULE SECTION — ALWAYS VISIBLE for non-running tasks -->
            <div style="border-top:1px solid var(--border);padding-top:12px">
              <div style="display:flex;align-items:center;gap:12px;margin-bottom:10px">
                <span class="heading-sm">Schedule</span>
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
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${scheduleSelectVal==='once'?'border:2px solid var(--accent)':''}" onclick="OL.quickSchedule('${esc(t.id)}','once')">Now</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;background:var(--bg-input);${scheduleSelectVal==='in:5m'?'border:2px solid var(--accent)':''}" onclick="OL.quickSchedule('${esc(t.id)}','in:5m')">In 5m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;background:var(--bg-input);${scheduleSelectVal==='in:15m'?'border:2px solid var(--accent)':''}" onclick="OL.quickSchedule('${esc(t.id)}','in:15m')">In 15m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;background:var(--bg-input);${scheduleSelectVal==='in:30m'?'border:2px solid var(--accent)':''}" onclick="OL.quickSchedule('${esc(t.id)}','in:30m')">In 30m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;background:var(--bg-input);${scheduleSelectVal==='in:1h'?'border:2px solid var(--accent)':''}" onclick="OL.quickSchedule('${esc(t.id)}','in:1h')">In 1h</button>
                <span style="font-size:11px;color:var(--text-muted)">At:</span>
                <input type="datetime-local" id="task-sched-at-picker" class="conv-search" style="font-size:11px;padding:4px" value="${t.next_run_at ? new Date(t.next_run_at).toISOString().slice(0,16) : ''}" />
                <button class="btn-search btn-sm" onclick="OL.scheduleAt('${esc(t.id)}')">Set</button>
              </div>

              <!-- Recurring options -->
              <div id="task-sched-recurring" style="display:${!isOneShot ? 'flex' : 'none'};gap:6px;align-items:center;flex-wrap:wrap;margin-bottom:8px">
                <span style="font-size:11px;color:var(--text-muted)">Every:</span>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='5m'?'border:2px solid var(--green)':''}" onclick="OL.quickSchedule('${esc(t.id)}','5m')">5m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='15m'?'border:2px solid var(--green)':''}" onclick="OL.quickSchedule('${esc(t.id)}','15m')">15m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='30m'?'border:2px solid var(--green)':''}" onclick="OL.quickSchedule('${esc(t.id)}','30m')">30m</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='1h'?'border:2px solid var(--green)':''}" onclick="OL.quickSchedule('${esc(t.id)}','1h')">1h</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='6h'?'border:2px solid var(--green)':''}" onclick="OL.quickSchedule('${esc(t.id)}','6h')">6h</button>
                <button class="btn-search" style="font-size:11px;padding:4px 10px;${t.schedule==='24h'?'border:2px solid var(--green)':''}" onclick="OL.quickSchedule('${esc(t.id)}','24h')">24h</button>
              </div>

              ${t.next_run_at ? `<div style="font-size:12px;color:var(--text-muted)">Next run: <strong style="color:var(--text-primary)">${new Date(t.next_run_at).toLocaleString()}</strong></div>` : ''}
            </div>
            ` : ''}
          </div>

          <!-- 3b. DEPENDS ON -->
          <div id="task-dependency-section" style="margin-bottom:16px;padding:12px;border:1px solid var(--border);border-radius:8px;background:var(--bg-secondary)">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span class="heading-sm">Depends On</span>
              ${t.depends_on ? `<span class="badge scope badge-sm">CHAINED</span>` : `<span style="font-size:11px;color:var(--text-muted)">No dependency</span>`}
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
              <button id="task-dep-set-btn" class="btn-search" style="font-size:11px;padding:4px 12px;display:none" onclick="OL.setDependency('${esc(t.id)}')">Set</button>
              ${t.depends_on ? `<button class="btn-dismiss" style="font-size:11px;padding:4px 12px" onclick="OL.removeDependency('${esc(t.id)}')">Remove</button>` : ''}
            </div>
          </div>

          <!-- 4. INSTRUCTIONS (editable) -->
          <div id="task-instructions-section" style="margin-bottom:12px">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
              <span class="heading-sm">Instructions</span>
              <button id="task-instructions-edit-btn" class="btn-search btn-xs" onclick="OL.editInstructions('${esc(t.id)}')">${t.description ? 'Edit' : 'Add Instructions'}</button>
            </div>
            <div id="task-instructions-display">
              ${t.description
                ? `<div style="font-size:13px;color:var(--text-secondary);padding:10px 12px;border:1px solid var(--border);border-radius:6px;background:var(--bg-secondary);white-space:pre-wrap;word-break:break-word;font-family:var(--font-mono);line-height:1.5;max-height:400px;overflow-y:auto">${esc(t.description)}</div>`
                : `<div style="font-size:12px;color:var(--text-muted);font-style:italic">No instructions</div>`}
            </div>
            <div id="task-instructions-editor" style="display:none">
              <textarea id="task-instructions-textarea" style="width:100%;min-height:300px;padding:12px;font-size:13px;line-height:1.5;font-family:var(--font-mono);resize:both;border:1px solid var(--accent);border-radius:6px;background:var(--bg-secondary);color:var(--text-primary);box-sizing:border-box">${esc(t.description || '')}</textarea>
              <div style="display:flex;gap:8px;margin-top:8px">
                <button class="btn-search" style="padding:4px 14px;font-size:12px" onclick="OL.saveInstructions('${esc(t.id)}')">Save</button>
                <button class="btn-dismiss" style="padding:4px 14px;font-size:12px" onclick="OL.cancelEditInstructions()">Cancel</button>
              </div>
            </div>
          </div>

          <!-- EXECUTION CONTROLS -->
          <div id="task-knobs-mount" style="margin-top:16px"></div>

          <!-- COMMENTS -->
          <div id="task-comments-mount" style="margin-top:16px"></div>

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
            <div class="section-label">Output</div>
            <div id="task-parsed-output" class="scroll-detail"></div>
            <div style="margin-top:6px">
              <span role="button" tabindex="0" style="font-size:11px;color:var(--text-muted);cursor:pointer;text-decoration:underline" onclick="var el=document.getElementById('task-raw-output');var v=el.style.display==='none';el.style.display=v?'':'none'" onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();this.click()}" aria-expanded="false">Toggle raw JSON</span>
            </div>
            <div class="action-response" id="task-raw-output" style="max-height:200px;overflow-y:auto;display:none;font-size:10px">${esc(t.last_output.length > 50000 ? t.last_output.substring(0, 50000) + '\n... (truncated for display, full output in DB)' : t.last_output)}</div>
          ` : ''}

          <!-- FOOTER -->
          <div style="margin-top:12px;font-family:var(--font-mono);font-size:10px;color:var(--text-muted)">
            Node: ${esc(t.source_node)} | ID: ${esc(t.id)}
          </div>
        </div>
      `;

      const knobMount = document.getElementById('task-knobs-mount');
      if (knobMount && OL.renderKnobSection) {
        OL.renderKnobSection(knobMount, { type: 'task', id: t.id });
      }

      const commentsMount = document.getElementById('task-comments-mount');
      if (commentsMount && OL.renderCommentsSection) {
        OL.renderCommentsSection(commentsMount, { type: 'task', id: t.id });
      }

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
        OL.onView(el, 'click', () => {
          OL.switchView('manifests');
          setTimeout(() => OL.loadManifest(mid), 300);
        });
      });

      // Action cross-links
      bodyEl.querySelectorAll('.action-link').forEach(el => {
        OL.onView(el, 'click', () => {
          OL.switchView('actions');
          setTimeout(() => OL.loadActionDetail(el.dataset.aid), 300);
        });
      });

      // Run history click — show output for that run
      bodyEl.querySelectorAll('.task-run-item').forEach(el => {
        OL.onView(el, 'click', () => {
          const runId = el.dataset.runId;
          const taskId = el.dataset.taskId;
          loadRunOutput(taskId, runId, el);
        });
      });

      // Schedule mode radio toggle — show/hide one-shot vs recurring options
      bodyEl.querySelectorAll('input[name="task-sched-mode"]').forEach(radio => {
        OL.onView(radio, 'change', () => {
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
                <span style="font-size:12px;font-weight:600;color:var(--text-primary);cursor:pointer;text-decoration:underline" onclick="OL.loadTaskDetail('${esc(dep.id)}')">${esc(dep.title)}</span>
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

          OL.onView(depSearch, 'input', () => populateOptions(depSearch.value));
        }
      })();

      // Parse output into readable format
      if (t.last_output && t.status !== 'running' && t.status !== 'paused') {
        const parsedEl = document.getElementById('task-parsed-output');
        if (parsedEl) {
          parsedEl.innerHTML = OL.parseTaskOutput(t.last_output);
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
              liveEl.innerHTML = OL.parseTaskOutput(data.lines.join('\n'));
              liveEl.scrollTop = liveEl.scrollHeight;
            } else {
              liveEl.innerHTML = '<span style="color:var(--text-muted)">Waiting for output...</span>';
            }
            if (data && data.running) {
              OL.viewTimeout(pollOutput, 2000);
            } else {
              // Task finished — DO NOT full-reload the detail. That used
              // to cause flicker: the tasks.status column lags the
              // /output endpoint's running flag, so loadTaskDetail
              // re-rendered with isRunningOrPaused=true, which restarted
              // pollOutput, which saw running=false again, and the
              // whole detail panel flickered at poll cadence. Instead
              // we just stop polling and refresh the task list so the
              // row's status updates naturally; a manual click into
              // the task brings in the freshly-completed detail.
              OL.loadTasks();
            }
          } catch (e) {}
        };
        pollOutput();
      }
    } catch (e) {
      console.error('Load task detail failed:', e);
    }
  };

  OL.showTaskCreateForm = async function() {
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
          <label class="form-label">Manifest <span style="opacity:0.5">(optional — leave blank for standalone task)</span></label>
          <input type="text" id="tc-manifest-search" class="conv-search" placeholder="Search manifests..." style="font-size:13px;margin-bottom:6px" />
          <select id="tc-manifest-id" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
            <option value="">No manifest (standalone)</option>
            ${manifestOptions}
          </select>
        </div>
        <div style="margin-bottom:16px">
          <label class="form-label">Task Title</label>
          <input type="text" id="tc-title" class="conv-search" placeholder="What should the agent do?" style="font-size:13px" />
        </div>
        <div style="margin-bottom:16px">
          <label class="form-label">Instructions</label>
          <textarea id="tc-description" class="conv-search" placeholder="What should the agent do? These instructions will be executed exactly as written..." style="font-size:14px;min-height:300px;width:100%;resize:both;font-family:monospace;padding:12px;line-height:1.5;box-sizing:border-box"></textarea>
          <div id="tc-instructions-error" style="font-size:12px;color:var(--red);margin-top:4px;display:none">Instructions are required — the agent needs to know what to do</div>
        </div>
        <div style="display:flex;gap:12px;margin-bottom:16px">
          <div style="flex:1">
            <label class="form-label">Schedule</label>
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
            <label class="form-label">Agent</label>
            <select id="tc-agent" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
              <option value="claude-code">Claude Code</option>
              <option value="cursor">Cursor</option>
              <option value="copilot">Copilot</option>
              <option value="custom">Custom</option>
            </select>
          </div>
        </div>
        <div id="tc-datetime-row" style="margin-bottom:16px">
          <label class="form-label">Run At</label>
          <input type="datetime-local" id="tc-run-at" class="conv-search" style="font-size:13px" />
        </div>
        <div class="flex-gap">
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
    OL.onView(searchInput, 'input', () => {
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
      const instrErr = bodyEl.querySelector('#tc-instructions-error');
      if (!desc) { instrErr.style.display = 'block'; return; } else { instrErr.style.display = 'none'; }
      if (!manifestId && !title) { bodyEl.querySelector('#tc-status').textContent = 'Title is required for standalone tasks'; return; }

      let schedule = sched;
      if (sched === 'at') {
        const dt = bodyEl.querySelector('#tc-run-at').value;
        if (!dt) { bodyEl.querySelector('#tc-status').textContent = 'Set a date/time'; return; }
        schedule = 'at:' + new Date(dt).toISOString();
      }

      const resp = await fetch('/api/tasks', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({manifest_id: manifestId, title, description: desc, schedule, agent})
      });
      if (!resp.ok) {
        bodyEl.querySelector('#tc-status').textContent = 'Error: ' + resp.status;
        return;
      }
      const newTask = await resp.json();
      bodyEl.querySelector('#tc-status').textContent = 'Created!';
      OL.loadTasks();
      if (newTask && newTask.id) {
        setTimeout(() => OL.loadTaskDetail(newTask.id), 300);
      } else {
        setTimeout(() => {
          titleEl.textContent = 'Select a task';
          bodyEl.innerHTML = '<div class="empty-state">Task created. Click it in the tree to view details.</div>';
        }, 1000);
      }
    };
  };

  // Internal helper — load and display output for a specific run
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

  // Window globals for onclick handlers

  OL.setDependency = async function(taskId) {
    const sel = document.getElementById('task-dep-select');
    if (!sel || !sel.value) return;
    await fetch('/api/tasks/' + taskId + '/dependency', {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({depends_on: sel.value})
    });
    OL.loadTaskDetail(taskId);
    OL.loadTasks();
  };

  OL.removeDependency = async function(taskId) {
    await fetch('/api/tasks/' + taskId + '/dependency', {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({depends_on: ''})
    });
    OL.loadTaskDetail(taskId);
    OL.loadTasks();
  };

  OL.taskAction = async function(id, action) {
    await fetch('/api/tasks/' + id + '/' + action, {method: 'POST'});
    OL.loadTaskDetail(id);
    OL.loadTasks();
  };

  OL.taskStart = async function(id) {
    await fetch('/api/tasks/' + id + '/start', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({schedule: 'once'})
    });
    OL.loadTaskDetail(id);
    OL.loadTasks();
  };

  // Quick schedule — one click reschedule to a preset
  OL.quickSchedule = async function(id, schedule) {
    await fetch('/api/tasks/' + id + '/reschedule', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({schedule})
    });
    OL.loadTaskDetail(id);
    OL.loadTasks();
  };

  // Schedule at a specific datetime from the picker
  OL.scheduleAt = async function(id) {
    const dt = document.getElementById('task-sched-at-picker');
    if (!dt || !dt.value) { alert('Pick a date/time first'); return; }
    const schedule = 'at:' + new Date(dt.value).toISOString();
    await fetch('/api/tasks/' + id + '/reschedule', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({schedule})
    });
    OL.loadTaskDetail(id);
    OL.loadTasks();
  };

  // Reschedule — called from the Reschedule button in control bar
  OL.rescheduleTask = async function(id) {
    // Just scroll to the schedule section — it's always visible
    const schedSection = document.querySelector('#task-sched-oneshot, #task-sched-recurring');
    if (schedSection) {
      schedSection.scrollIntoView({behavior: 'smooth', block: 'center'});
      schedSection.style.outline = '2px solid var(--accent)';
      setTimeout(() => { schedSection.style.outline = ''; }, 1500);
    }
  };

  // Edit instructions — toggle editor view
  OL.editInstructions = function(id) {
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
  OL.saveInstructions = async function(id) {
    const ta = document.getElementById('task-instructions-textarea');
    if (!ta) return;
    await fetch('/api/tasks/' + id, {
      method: 'PATCH',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({description: ta.value})
    });
    OL.loadTaskDetail(id);
  };

  // Cancel edit — revert to display view
  OL.cancelEditInstructions = function() {
    const display = document.getElementById('task-instructions-display');
    const editor = document.getElementById('task-instructions-editor');
    const btn = document.getElementById('task-instructions-edit-btn');
    if (display) display.style.display = '';
    if (editor) editor.style.display = 'none';
    if (btn) btn.style.display = '';
  };

  OL.taskArchive = async function(id) {
    await fetch('/api/tasks/' + id + '/cancel', {method: 'POST'});
    OL.loadTasks();
    OL.loadTaskDetail(id);
  };

})(window.OL);
