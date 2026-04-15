(function(OL) {
  'use strict';
  var esc = OL.esc;

  OL.renderMarkers = function(markers) {
    var panel = document.getElementById('markers-panel');
    var el = document.getElementById('overview-markers');
    var badge = document.getElementById('marker-count-badge');
    if (!markers.length) {
      if (panel) panel.style.display = 'none';
      return;
    }
    if (panel) panel.style.display = '';
    if (badge) badge.textContent = markers.length;
    el.innerHTML = markers.map(function(m) {
      var priorityClass = m.priority === 'urgent' ? 'marker-priority-urgent' :
                           m.priority === 'high' ? 'marker-priority-high' : '';
      return '<div class="marker-item ' + priorityClass + '">' +
        '<div><span class="marker-from">' + esc(m.from_node) + '</span> flagged a ' + esc(m.target_type) + '</div>' +
        '<div class="marker-message">' + esc(m.message) + '</div>' +
        '<div class="marker-target">' + esc(m.target_path || m.target_id) + '</div>' +
        '<div class="marker-actions">' +
          '<button onclick="window._markerDone(\'' + esc(m.id) + '\')">Done</button>' +
          '<button onclick="window._markerSeen(\'' + esc(m.id) + '\')">Seen</button>' +
        '</div>' +
      '</div>';
    }).join('');
  };

  window._markerDone = async function(id) {
    await fetch('/api/markers/' + id + '/done', {method: 'POST'});
    OL.refreshAll();
  };

  window._markerSeen = async function(id) {
    await fetch('/api/markers/' + id + '/seen', {method: 'POST'});
    OL.refreshAll();
  };
})(window.OL);
