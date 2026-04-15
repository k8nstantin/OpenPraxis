(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  OL.renderActivities = async function() {
    var el = document.getElementById('activity-feed');
    try {
      var peerGroups = await fetchJSON('/api/activity/by-peer');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state">No activity yet</div>';
        return;
      }
      var html = '';
      for (var pi = 0; pi < peerGroups.length; pi++) {
        var pg = peerGroups[pi];
        html += '<div class="tree-node peer-header clickable" data-act-peer="' + pi + '" role="button" tabindex="0" aria-expanded="true">';
        html += '<span class="tree-arrow">\u25BC</span>';
        html += '<span class="status-dot green"></span>';
        html += '<span>' + esc(pg.peer_id) + '</span>';
        html += '<span class="count">' + pg.count + '</span>';
        html += '</div>';
        html += '<div class="peer-children" data-act-peer-children="' + pi + '">';
        for (var si = 0; si < pg.sessions.length; si++) {
          var sg = pg.sessions[si];
          html += '<div class="tree-node session-header clickable" data-act-session="' + pi + '-' + si + '" role="button" tabindex="0" aria-expanded="true">';
          html += '<span class="tree-arrow badge-sm">\u25BC</span>';
          html += '<span class="status-dot green dot-sm"></span>';
          html += '<span>' + esc(sg.session) + '</span>';
          html += '<span class="count">' + sg.count + '</span>';
          html += '</div>';
          html += '<div class="session-children" data-act-session-children="' + pi + '-' + si + '">';
          for (var ai = 0; ai < sg.activities.length; ai++) {
            var a = sg.activities[ai];
            var icon = a.type === 'memory' ? '\u25CF' : '\u2631';
            var badge = a.type === 'memory'
              ? '<span class="badge type badge-sm">memory</span>'
              : '<span class="badge scope badge-sm">conversation</span>';
            html += '<div class="activity-item clickable" data-activity-type="' + esc(a.type) + '" data-activity-id="' + esc(a.id) + '" role="button" tabindex="0">';
            html += '<span class="activity-time">' + formatTime(a.time) + '</span>';
            html += '<span class="activity-icon">' + icon + '</span>';
            html += badge;
            html += '<span class="activity-text">' + esc(a.title) + '</span>';
            html += '</div>';
          }
          html += '</div>';
        }
        html += '</div>';
      }
      el.innerHTML = html;

      OL.wireTreeToggles(el, 'data-act-peer');
      OL.wireTreeToggles(el, 'data-act-session');

      // Activity item click — cross-navigate
      el.querySelectorAll('.activity-item').forEach(function(item) {
        var handler = function() {
          var type = item.dataset.activityType;
          var id = item.dataset.activityId;
          if (type === 'conversation') {
            OL.switchView('conversations');
            setTimeout(function() { OL.loadConv(id); }, 300);
          } else if (type === 'memory') {
            OL.switchView('memories');
            setTimeout(function() { OL.loadMemoryPeerDetail(id); }, 300);
          }
        };
        OL.onView(item, 'click', handler);
        OL.onView(item, 'keydown', function(e) {
          if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handler(); }
        });
      });
    } catch (e) {
      console.error('Load activity failed:', e);
    }
  };
})(window.OL);
