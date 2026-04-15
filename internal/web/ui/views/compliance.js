(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  // --- Delusions ---
  OL.loadDelusions = async function() {
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

      OL.wireTreeToggles(el, 'data-del-peer');

      // Action cross-links
      el.querySelectorAll('.action-link').forEach(link => {
        link.addEventListener('click', () => {
          OL.switchView('actions');
          setTimeout(() => OL.loadActionDetail(link.dataset.aid), 300);
        });
      });
    } catch (e) {
      console.error('Load delusions failed:', e);
    }
  };

  window._delusionAction = async function(id, action) {
    await fetch('/api/delusions/' + id + '/' + action, {method: 'POST'});
    OL.loadDelusions();
  };

  OL.updateDelusionCount = async function() {
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
  };

  // --- Amnesia ---
  OL.loadAmnesia = async function() {
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

      OL.wireTreeToggles(el, 'data-amn-peer');
      OL.wireTreeToggles(el, 'data-amn-session');

      // Action cross-links
      el.querySelectorAll('.action-link').forEach(link => {
        link.addEventListener('click', () => {
          OL.switchView('actions');
          setTimeout(() => OL.loadActionDetail(link.dataset.aid), 300);
        });
      });
    } catch (e) {
      console.error('Load amnesia failed:', e);
    }
  };

  window._amnesiaAction = async function(id, action) {
    await fetch('/api/amnesia/' + id + '/' + action, {method: 'POST'});
    OL.loadAmnesia();
  };

  OL.updateAmnesiaCount = async function() {
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
  };

  // --- Visceral ---
  OL.loadVisceral = async function() {
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
      OL.loadVisceral();
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

      OL.wireTreeToggles(el, 'data-visc-peer');
    } catch (e) {
      console.error('Load visceral failed:', e);
    }
  };

  window._deleteVisceral = async function(id) {
    if (!confirm('Remove this visceral rule?')) return;
    await fetch('/api/visceral/' + id, {method: 'DELETE'});
    OL.loadVisceral();
  };
})(window.OL);
