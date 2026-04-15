(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  function wireActionLinks(container) {
    container.querySelectorAll('.action-link').forEach(function(link) {
      OL.onView(link, 'click', function() {
        OL.switchView('actions');
        setTimeout(function() { OL.loadActionDetail(link.dataset.aid); }, 300);
      });
    });
  }

  // --- Delusions ---
  OL.loadDelusions = async function() {
    var el = document.getElementById('delusion-list');
    try {
      var peerGroups = await fetchJSON('/api/delusions/by-peer');

      OL.renderTree(el, peerGroups, {
        prefix: 'del',
        emptyMessage: 'No delusions detected. Agents are following their manifests.',
        levels: [{
          label: function(pg) { return esc(pg.peer_id); },
          count: function(pg) { return pg.count; },
          children: function(pg) { return pg.delusions; },
        }],
        renderLeaf: function(d) {
          var sid = d.session_id ? d.session_id.substring(0, 12) : '';
          var simPercent = Math.round(d.score * 100);
          var statusClass = d.status === 'confirmed' ? 'confirmed' : d.status === 'dismissed' ? 'dismissed' : 'flagged';

          return '<div class="amnesia-item ' + statusClass + '">' +
            '<div class="amnesia-header">' +
              '<span class="amnesia-score medium">' + simPercent + '% relevant</span>' +
              '<span class="badge type">' + esc(d.tool_name) + '</span>' +
              (d.action_id ? '<span class="badge scope action-link" style="cursor:pointer;font-size:10px" data-aid="' + esc(d.action_id) + '">view action</span>' : '') +
              '<span class="amnesia-status-label">' + esc(d.status) + '</span>' +
              '<span style="color:var(--text-muted);font-size:11px;margin-left:auto">' + formatTime(d.created_at) + '</span>' +
            '</div>' +
            '<div class="amnesia-session">' +
              '<span style="color:var(--text-muted);font-size:12px">Session:</span>' +
              '<span class="session-uuid">' + esc(sid) + '</span>' +
            '</div>' +
            '<div class="amnesia-rule">' +
              '<span style="color:var(--yellow);font-weight:500">Manifest [' + esc(d.manifest_marker) + ']:</span> ' + esc(d.manifest_title) +
            '</div>' +
            '<div style="font-size:12px;color:var(--text-secondary);margin-bottom:6px">' + esc(d.reason) + '</div>' +
            '<div class="amnesia-action">' +
              '<span style="color:var(--text-muted)">Action:</span> ' + esc(d.tool_input) +
            '</div>' +
            (d.status === 'flagged' ? '<div class="amnesia-actions">' +
              '<button class="btn-confirm" onclick="window._delusionAction(' + d.id + ',\'confirm\')">Confirm Off-Spec</button>' +
              '<button class="btn-dismiss" onclick="window._delusionAction(' + d.id + ',\'dismiss\')">Dismiss</button>' +
            '</div>' : '') +
          '</div>';
        },
        afterRender: function(container) { wireActionLinks(container); },
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
      var events = await fetchJSON('/api/delusions?status=flagged');
      var badge = document.getElementById('delusion-badge');
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
    var el = document.getElementById('amnesia-list');
    try {
      var peerGroups = await fetchJSON('/api/amnesia/by-peer');

      // Transform: group events by session within each peer
      var treeData = (peerGroups || []).map(function(pg) {
        var sessionMap = {};
        var sessionOrder = [];
        for (var i = 0; i < pg.events.length; i++) {
          var a = pg.events[i];
          var sid = a.session_id || 'unknown';
          if (!sessionMap[sid]) {
            sessionMap[sid] = { sid: sid, events: [], taskId: '' };
            sessionOrder.push(sid);
          }
          sessionMap[sid].events.push(a);
          if (!sessionMap[sid].taskId && a.task_id) sessionMap[sid].taskId = a.task_id;
        }
        return {
          peer_id: pg.peer_id,
          count: pg.count,
          sessions: sessionOrder.map(function(s) { return sessionMap[s]; }),
        };
      });

      OL.renderTree(el, treeData, {
        prefix: 'amn',
        emptyMessage: 'No amnesia events',
        levels: [
          {
            label: function(pg) { return esc(pg.peer_id); },
            count: function(pg) { return pg.count; },
            children: function(pg) { return pg.sessions; },
          },
          {
            dotColor: '',
            expanded: false,
            extra: function(sg) {
              var shortSid = sg.sid.length > 12 ? sg.sid.substring(0, 12) : sg.sid;
              var shortTask = sg.taskId ? sg.taskId.substring(0, 8) : '';
              return '<span class="session-uuid">' + esc(shortSid) + '</span>' +
                (shortTask ? '<span class="badge scope" style="font-size:10px">' + esc(shortTask) + '</span>' : '');
            },
            count: function(sg) { return sg.events.length; },
            children: function(sg) { return sg.events; },
          }
        ],
        renderLeaf: function(a) {
          var scorePercent = Math.round(a.score * 100);
          var scoreClass = scorePercent >= 80 ? 'high' : scorePercent >= 60 ? 'medium' : 'low';
          var statusClass = a.status === 'confirmed' ? 'confirmed' : a.status === 'dismissed' ? 'dismissed' : 'flagged';

          return '<div class="amnesia-item ' + statusClass + '" style="margin-left:24px">' +
            '<div class="amnesia-header">' +
              '<span class="amnesia-score ' + scoreClass + '">' + scorePercent + '%</span>' +
              '<span class="badge type">' + esc(a.tool_name) + '</span>' +
              (a.action_id ? '<span class="badge scope action-link" style="cursor:pointer;font-size:10px" data-aid="' + esc(a.action_id) + '">view action</span>' : '') +
              '<span class="amnesia-status-label">' + esc(a.status) + '</span>' +
              '<span style="color:var(--text-muted);font-size:11px;margin-left:auto">' + formatTime(a.created_at) + '</span>' +
            '</div>' +
            '<div class="amnesia-rule">' +
              '<span style="color:var(--red);font-weight:500">Rule [' + esc(a.rule_marker) + ']:</span> ' + esc(a.rule_text) +
            '</div>' +
            '<div class="amnesia-action">' +
              '<span style="color:var(--text-muted)">Action:</span> ' + esc(a.tool_input) +
            '</div>' +
            (a.status === 'flagged' ? '<div class="amnesia-actions">' +
              '<button class="btn-confirm" onclick="window._amnesiaAction(' + a.id + ',\'confirm\')">Confirm Violation</button>' +
              '<button class="btn-dismiss" onclick="window._amnesiaAction(' + a.id + ',\'dismiss\')">Dismiss</button>' +
            '</div>' : '') +
          '</div>';
        },
        afterRender: function(container) { wireActionLinks(container); },
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
      var events = await fetchJSON('/api/amnesia?status=flagged');
      var badge = document.getElementById('amnesia-badge');
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
    var el = document.getElementById('visceral-rules');
    var input = document.getElementById('visceral-input');
    var btn = document.getElementById('visceral-add-btn');

    btn.onclick = async function() {
      var rule = input.value.trim();
      if (!rule) return;
      await fetch('/api/visceral', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({rule: rule})
      });
      input.value = '';
      OL.loadVisceral();
    };
    input.onkeypress = function(e) { if (e.key === 'Enter') btn.click(); };

    try {
      var peerGroups = await fetchJSON('/api/visceral/by-peer');
      var ruleNum = 0;

      OL.renderTree(el, peerGroups, {
        prefix: 'visc',
        emptyMessage: 'No visceral rules set yet. Add rules that every agent must follow.',
        levels: [{
          label: function(pg) { return esc(pg.peer_id); },
          count: function(pg) { return pg.count; },
          children: function(pg) { return pg.rules; },
        }],
        renderLeaf: function(r) {
          ruleNum++;
          return '<div class="visceral-rule" style="padding-left:24px">' +
            '<span class="visceral-num">' + ruleNum + '</span>' +
            '<span class="session-uuid">' + esc(r.marker) + '</span>' +
            '<span class="visceral-text">' + esc(r.text) + '</span>' +
            '<span style="color:var(--text-muted);font-size:11px">' + esc(r.source || '') + '</span>' +
            '<button class="btn-copy" onclick="window._copy(\'recall memory ' + esc(r.marker) + '\')" title="Copy reference">&#x2398;</button>' +
            '<button class="visceral-delete" onclick="window._deleteVisceral(\'' + esc(r.id) + '\')" title="Remove rule">&times;</button>' +
          '</div>';
        },
      });
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
