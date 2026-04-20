(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc;
  var _pendingIdeaId = null;

  function renderIdeaSearchList(el, ideas) {
    if (!ideas || !ideas.length) {
      el.innerHTML = '<div class="empty-state" style="padding:16px">No ideas found</div>';
      return;
    }
    el.innerHTML = ideas.map(function(i) {
      var prioClass = i.priority === 'critical' || i.priority === 'high' ? 'high' :
        i.priority === 'low' ? 'low' : 'medium';
      return '<div class="manifest-item clickable" data-id="' + esc(i.id) + '" role="button" tabindex="0" ' +
        'onclick="OL.loadIdea(\'' + esc(i.id) + '\')" ' +
        'onkeydown="if(event.key===\'Enter\'||event.key===\' \'){event.preventDefault();this.click()}">' +
        '<div style="display:flex;align-items:center;gap:6px;margin-bottom:4px">' +
          '<span class="amnesia-score ' + prioClass + ' badge-sm">' + esc(i.priority) + '</span>' +
          '<span class="session-uuid">' + esc(i.marker) + '</span>' +
          '<span class="badge">' + esc(i.status) + '</span>' +
        '</div>' +
        '<div class="manifest-item-title">' + esc(i.title) + '</div>' +
        (i.description ? '<div style="font-size:12px;color:var(--text-secondary)">' + esc(i.description) + '</div>' : '') +
      '</div>';
    }).join('');
  }

  OL.loadIdeas = async function() {
    var el = document.getElementById('ideas-list');
    var btn = document.getElementById('idea-add-btn');
    var titleInput = document.getElementById('idea-title');
    var prioritySelect = document.getElementById('idea-priority');

    var mount = document.getElementById('ideas-search-mount');
    if (mount && OL.mountSearchInput) {
      OL.mountSearchInput(mount, {
        placeholder: 'Search ideas by id, marker, or keyword...',
        onSearch: async function(q) {
          var results = await fetchJSON('/api/ideas/search?q=' + encodeURIComponent(q));
          renderIdeaSearchList(el, results || []);
          return (results || []).length;
        },
        onClear: function() { OL.loadIdeas(); }
      });
    }

    btn.onclick = async function() {
      var title = titleInput.value.trim();
      if (!title) return;
      await fetch('/api/ideas', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({title: title, priority: prioritySelect.value})
      });
      titleInput.value = '';
      OL.loadIdeas();
    };
    titleInput.onkeypress = function(e) { if (e.key === 'Enter') btn.click(); };

    try {
      var peerGroups = await fetchJSON('/api/ideas/by-peer');
      if (!peerGroups || !peerGroups.length) {
        el.innerHTML = '<div class="empty-state" style="padding:16px">No ideas yet</div>';
        return;
      }
      var html = '';
      var allIdeas = [];
      for (var pi = 0; pi < peerGroups.length; pi++) {
        var pg = peerGroups[pi];
        html += '<div class="tree-node peer-header clickable" data-idea-peer="' + pi + '" role="button" tabindex="0" aria-expanded="true">' +
          '<span class="tree-arrow">&#x25BC;</span>' +
          '<span class="status-dot green"></span>' +
          '<span>' + esc(pg.peer_id) + '</span>' +
          '<span class="count">' + pg.count + '</span>' +
        '</div>';
        html += '<div class="peer-children" data-idea-peer-children="' + pi + '">';
        for (var ii = 0; ii < pg.ideas.length; ii++) {
          var i = pg.ideas[ii];
          allIdeas.push(i);
          var prioClass = i.priority === 'critical' || i.priority === 'high' ? 'high' : i.priority === 'low' ? 'low' : 'medium';
          html += '<div class="manifest-item clickable" data-id="' + esc(i.id) + '" role="button" tabindex="0">' +
            '<div style="display:flex;align-items:center;gap:6px;margin-bottom:4px">' +
              '<span class="amnesia-score ' + prioClass + ' badge-sm">' + esc(i.priority) + '</span>' +
              '<span class="session-uuid">' + esc(i.marker) + '</span>' +
              '<span class="badge">' + esc(i.status) + '</span>' +
            '</div>' +
            '<div class="manifest-item-title">' + esc(i.title) + '</div>' +
          '</div>';
        }
        html += '</div>';
      }
      el.innerHTML = html;

      OL.wireTreeToggles(el, 'data-idea-peer');

      el.querySelectorAll('.manifest-item').forEach(function(item) {
        OL.onView(item, 'click', function() { OL.loadIdea(item.dataset.id); });
        OL.onView(item, 'keydown', function(e) {
          if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); OL.loadIdea(item.dataset.id); }
        });
      });

      if (_pendingIdeaId) {
        var id = _pendingIdeaId;
        _pendingIdeaId = null;
        var match = allIdeas.find(function(i) { return i.id.startsWith(id) || i.marker === id; });
        if (match) OL.loadIdea(match.id);
      }
    } catch (e) {
      console.error('Load ideas failed:', e);
    }
  };

  OL.loadIdea = async function(id) {
    document.querySelectorAll('#ideas-list .manifest-item').forEach(function(i) { i.classList.remove('active'); });
    var active = document.querySelector('#ideas-list .manifest-item[data-id="' + id + '"]');
    if (active) active.classList.add('active');

    try {
      var idea = await fetchJSON('/api/ideas/' + id);
      if (!idea) return;

      var titleEl = document.getElementById('idea-detail-title');
      var bodyEl = document.getElementById('idea-detail');
      var prioClass = idea.priority === 'critical' || idea.priority === 'high' ? 'high' : idea.priority === 'low' ? 'low' : 'medium';

      titleEl.textContent = idea.title;

      // Fetch linked manifests
      var linkedHtml = '';
      try {
        var linked = await fetchJSON('/api/ideas/' + idea.id + '/manifests');
        if (linked && linked.length) {
          linkedHtml = '<div style="margin-bottom:12px"><span style="color:var(--text-muted);font-size:12px">Manifests:</span> ' +
            linked.map(function(m) { return '<span class="badge type manifest-link" style="cursor:pointer" data-mid="' + m.id + '">' + esc(m.marker) + ' ' + esc(m.title) + '</span>'; }).join(' ') +
            '</div>';
        }
      } catch(e) {}

      bodyEl.innerHTML =
        '<div class="manifest-detail-view">' +
          '<!-- BREADCRUMB -->' +
          '<div class="breadcrumb">' +
            '<span class="breadcrumb-link" onclick="OL.switchView(\'ideas\')">' + esc(idea.source_node ? idea.source_node.substring(0,12) : 'node') + '</span>' +
            '<span class="breadcrumb-sep"> &rarr; </span>' +
            '<span style="color:var(--text-primary)">' + esc(idea.marker) + ' ' + esc(idea.title) + '</span>' +
          '</div>' +
          '<div class="manifest-meta">' +
            '<span class="session-uuid" style="font-size:14px">' + esc(idea.marker) + '</span>' +
            '<span class="amnesia-score ' + prioClass + '">' + esc(idea.priority) + '</span>' +
            '<span class="badge">' + esc(idea.status) + '</span>' +
            '<span style="font-size:12px;color:var(--text-muted)">by ' + esc(idea.author) + '</span>' +
            '<button class="btn-copy" onclick="OL.copy(\'get idea ' + idea.marker + '\')" title="Copy ref" aria-label="Copy reference">&#x2398;</button>' +
          '</div>' +
          (idea.description ? '<div style="font-size:14px;color:var(--text-primary);margin:12px 0;line-height:1.5">' + esc(idea.description) + '</div>' : '') +
          linkedHtml +
          '<div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border);font-size:11px;color:var(--text-muted)">' +
            'Created: ' + new Date(idea.created_at).toLocaleString() + ' | ID: ' + esc(idea.id) +
          '</div>' +
          '<div style="margin-top:12px;display:flex;gap:8px">' +
            '<button class="btn-search promote-idea-btn btn-md">Create Manifest from Idea</button>' +
            '<button class="btn-dismiss" onclick="OL.archiveIdea(\'' + esc(idea.id) + '\')">Archive</button>' +
          '</div>' +
        '</div>';

      // Bind manifest links — click to navigate to manifest
      bodyEl.querySelectorAll('.manifest-link').forEach(function(el) {
        OL.onView(el, 'click', function() {
          OL.switchView('manifests');
          setTimeout(function() { OL.loadManifest(el.dataset.mid); }, 300);
        });
      });

      // Promote idea to manifest
      OL.onView(bodyEl.querySelector('.promote-idea-btn'), 'click', function() {
        OL.promoteToManifest(
          idea.title,
          idea.description || '',
          '# ' + idea.title + '\n\n' + (idea.description || '') + '\n\nPromoted from idea [' + idea.marker + ']\nPriority: ' + idea.priority + '\nStatus: ' + idea.status
        );
      });
    } catch(e) {
      console.error('Load idea failed:', e);
    }
  };

  // Navigate from manifest → specific idea
  OL.goToIdea = function(marker) {
    _pendingIdeaId = marker;
    OL.switchView('ideas');
  };

  OL.archiveIdea = async function(id) {
    var idea = await fetchJSON('/api/ideas/' + id);
    if (idea) {
      await fetchJSON('/api/ideas/' + id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({status: 'archive'}) });
    }
    OL.loadIdeas();
  };
})(window.OL);
