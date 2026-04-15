(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  OL.loadActions = async function() {
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
        html += `<div class="tree-node peer-header clickable" data-action-peer="${pi}" role="button" tabindex="0" aria-expanded="true">
          <span class="tree-arrow">&#x25BC;</span>
          <span class="status-dot green"></span>
          <span>${esc(pg.peer_id)}</span>
          <span class="count">${pg.count}</span>
        </div>`;
        html += `<div class="peer-children" data-action-peer-children="${pi}">`;
        for (let si = 0; si < pg.sessions.length; si++) {
          const sg = pg.sessions[si];
          const sid = sg.session.length > 12 ? sg.session.substring(0, 12) : sg.session;
          html += `<div class="tree-node session-header clickable" data-action-session="${pi}-${si}" role="button" tabindex="0" aria-expanded="false">
            <span class="tree-arrow" style="font-size:10px">&#x25BC;</span>
            <span class="status-dot green" style="width:6px;height:6px"></span>
            <span>${esc(sid)}</span>
            <span class="count">${sg.count}</span>
          </div>`;
          html += `<div class="session-children" data-action-session-children="${pi}-${si}" style="display:none">`;
          for (const a of sg.actions) {
            html += `<div class="tree-node peer-leaf clickable" data-action-id="${esc(a.id)}" role="button" tabindex="0">
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

      OL.wireTreeToggles(el, 'data-action-peer');
      OL.wireTreeToggles(el, 'data-action-session');

      // Action leaf click — fetch full detail
      el.querySelectorAll('.tree-node.peer-leaf').forEach(node => {
        var handler = function(e) {
          if (e.target.closest('.tree-arrow')) return;
          el.querySelectorAll('.tree-node').forEach(n => n.classList.remove('active'));
          node.classList.add('active');
          OL.loadActionDetail(node.dataset.actionId);
        };
        OL.onView(node, 'click', handler);
        OL.onView(node, 'keydown', (e) => {
          if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handler(e); }
        });
      });
    } catch (e) {
      console.error('Load actions failed:', e);
    }
  };

  OL.loadActionDetail = async function(id) {
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
        OL.onView(el, 'click', () => {
          OL.switchView('conversations');
          setTimeout(() => OL.loadConv(el.dataset.convId), 300);
        });
      });

      // Bind task link
      bodyEl.querySelectorAll('.task-nav').forEach(el => {
        OL.onView(el, 'click', () => {
          OL.switchView('tasks');
          setTimeout(() => OL.loadTaskDetail(el.dataset.tid), 300);
        });
      });
    } catch (e) {
      console.error('Load action detail failed:', e);
    }
  };
})(window.OL);
