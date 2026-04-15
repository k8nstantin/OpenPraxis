(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc;

  OL.loadRecall = async function() {
    var el = document.getElementById('recall-list');
    try {
      var items = await fetchJSON('/api/recall');
      if (!items || !items.length) {
        el.innerHTML = '<div class="empty-state">No deleted items. Everything is still alive.</div>';
        return;
      }
      var groups = {};
      for (var i = 0; i < items.length; i++) {
        var item = items[i];
        if (!groups[item.type]) groups[item.type] = [];
        groups[item.type].push(item);
      }
      var typeIcons = {memory: '&#x25A1;', manifest: '&#x2637;', idea: '&#x2726;', task: '&#x23F0;'};
      var typeColors = {memory: 'var(--accent)', manifest: 'var(--green)', idea: 'var(--yellow)', task: 'var(--text-secondary)'};
      var html = '';
      var types = ['memory', 'manifest', 'idea', 'task'];
      for (var ti = 0; ti < types.length; ti++) {
        var type = types[ti];
        var list = groups[type];
        if (!list || !list.length) continue;
        html += '<div style="margin-bottom:16px">';
        html += '<div style="font-size:13px;font-weight:600;color:' + typeColors[type] + ';margin-bottom:8px;text-transform:capitalize">' + typeIcons[type] + ' ' + type + 's (' + list.length + ')</div>';
        for (var li = 0; li < list.length; li++) {
          var it = list[li];
          html += '<div style="display:flex;align-items:center;gap:8px;padding:8px 0;border-bottom:1px solid var(--border)">';
          html += '<span class="session-uuid">' + esc(it.marker) + '</span>';
          html += '<span style="font-size:13px;color:var(--text-primary);flex:1">' + esc(it.title) + '</span>';
          html += '<button class="btn-search recall-restore btn-sm" data-type="' + esc(it.type) + '" data-id="' + esc(it.id) + '">Restore</button>';
          html += '</div>';
        }
        html += '</div>';
      }
      el.innerHTML = html;
      el.querySelectorAll('.recall-restore').forEach(function(btn) {
        OL.onView(btn, 'click', async function() {
          var type = btn.dataset.type;
          var id = btn.dataset.id;
          await fetch('/api/recall/' + type + '/' + id + '/restore', {method: 'POST'});
          btn.textContent = 'Restored';
          btn.disabled = true;
          btn.style.background = 'var(--green)';
          setTimeout(function() { OL.loadRecall(); }, 1000);
        });
      });
    } catch (e) {
      console.error('Load recall failed:', e);
    }
  };
})(window.OL);
