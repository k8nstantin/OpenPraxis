(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  function renderMemorySearchList(el, results) {
    if (!results || !results.length) {
      el.innerHTML = '<div class="empty-state">No memories found</div>';
      return;
    }
    el.innerHTML = results.map(function(r) {
      var m = r;
      var score = (typeof r.score === 'number') ? r.score : null;
      var preview = (m.l0 || m.l1 || '').substring(0, 80);
      return '<div class="tree-node peer-leaf clickable" data-memory-id="' + esc(m.id) + '" ' +
        'role="button" tabindex="0" ' +
        'onclick="OL.loadMemoryPeerDetail(\'' + esc(m.id) + '\')" ' +
        'onkeydown="if(event.key===\'Enter\'||event.key===\' \'){event.preventDefault();this.click()}">' +
        '<span class="session-uuid">' + esc(m.marker || (m.id ? m.id.substring(0,12) : '')) + '</span>' +
        (score != null ? '<span class="badge badge-sm" style="color:var(--accent)">' + score.toFixed(2) + '</span>' : '') +
        '<span style="font-size:12px;color:var(--text-primary);flex:1">' + esc(preview) + '</span>' +
      '</div>';
    }).join('');
  }

  OL.loadMemoryPeerTree = async function() {
    var mount = document.getElementById('memories-search-mount');
    var tree = document.getElementById('memory-peer-tree');
    if (mount && OL.mountSearchInput) {
      OL.mountSearchInput(mount, {
        placeholder: 'Search memories by id, marker, path, or keyword...',
        onSearch: async function(q) {
          var results = await fetchJSON('/api/memories/search?q=' + encodeURIComponent(q));
          renderMemorySearchList(tree, results || []);
          return (results || []).length;
        },
        onClear: function() { OL.loadMemoryPeerTree(); }
      });
    }
    try {
      var peerGroups = await fetchJSON('/api/memories/by-peer');
      var el = document.getElementById('memory-peer-tree');

      OL.renderTree(el, peerGroups, {
        prefix: 'mem',
        emptyMessage: 'No peers',
        levels: [
          {
            label: function(pg) { return esc(pg.peer_id); },
            count: function(pg) { return pg.count; },
            children: function(pg) { return pg.sessions; },
          },
          {
            label: function(sg) { return esc(sg.session); },
            count: function(sg) { return sg.count; },
            children: function(sg) { return sg.memories; },
            dotColor: 'green',
            dotStyle: 'width:6px;height:6px',
            expanded: true,
          }
        ],
        renderLeaf: function(m) {
          return '<div class="tree-node peer-leaf clickable tree-leaf" data-memory-id="' + esc(m.id) + '">' +
            '<span class="session-uuid">' + esc(m.marker) + '</span>' +
            '<span style="font-size:12px;color:var(--text-primary);flex:1">' + esc(m.l0.length > 50 ? m.l0.substring(0, 50) + '...' : m.l0) + '</span>' +
            '<button class="btn-copy-sm" onclick="event.stopPropagation();OL.copy(\'recall memory ' + esc(m.marker) + '\')" title="Copy ref" aria-label="Copy reference">&#x2398;</button>' +
          '</div>';
        },
        leafSelector: '.tree-leaf',
        onLeafClick: function(node) {
          el.querySelectorAll('.tree-node').forEach(function(n) { n.classList.remove('active'); });
          node.classList.add('active');
          OL.loadMemoryPeerDetail(node.dataset.memoryId);
        },
      });
    } catch (e) {
      console.error('Load peer memories failed:', e);
    }
  };

  OL.loadMemoryPeerDetail = async function(id) {
    try {
      const mem = await fetchJSON('/api/memories/' + id);
      if (mem) renderMemoryPeerDetail(mem);
    } catch (e) {
      console.error('Load memory peer detail failed:', e);
    }
  };

  function renderMemoryPeerDetail(mem) {
    const detail = document.getElementById('memory-peer-detail');
    let currentTier = 'l2';
    const tierContent = { l0: mem.l0 || '', l1: mem.l1 || '', l2: mem.l2 || '' };
    const marker = mem.id ? mem.id.substring(0, 12) : '';
    const session = mem.source_agent || 'unknown';
    const node = mem.source_node || 'unknown';

    detail.innerHTML = `
      <div class="memory-detail-view">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px">
          <span class="session-uuid" style="font-size:14px">${esc(marker)}</span>
          <span style="font-family:var(--font-mono);font-size:12px;color:var(--accent);flex:1">${esc(mem.path)}</span>
          <button class="btn-copy" onclick="OL.copy('recall memory ${marker}')" title="Copy reference" aria-label="Copy reference">&#x2398;</button>
        </div>
        <div class="memory-meta">
          <span class="badge type">${esc(mem.type || 'insight')}</span>
          <span class="badge scope">${esc(mem.scope || 'project')}</span>
          ${mem.project ? `<span class="badge">${esc(mem.project)}</span>` : ''}
          ${mem.domain ? `<span class="badge">${esc(mem.domain)}</span>` : ''}
          <span class="badge" style="color:var(--green)">${esc(session)}</span>
          <span class="badge" style="color:var(--accent)">${esc(node)}</span>
        </div>
        <div class="tier-tabs" role="tablist">
          <div class="tier-tab" data-tier="l0" role="tab" tabindex="0" aria-selected="false">L0 — One-liner</div>
          <div class="tier-tab" data-tier="l1" role="tab" tabindex="0" aria-selected="false">L1 — Summary</div>
          <div class="tier-tab active" data-tier="l2" role="tab" tabindex="0" aria-selected="true">L2 — Full</div>
        </div>
        <div class="memory-content" id="memory-peer-content-text">${esc(tierContent.l2)}</div>
        <div class="memory-timestamps" style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border);font-size:11px;color:var(--text-muted);display:flex;gap:16px;flex-wrap:wrap">
          <span>Created: ${mem.created_at ? new Date(mem.created_at).toLocaleString() : '-'}</span>
          <span>Updated: ${mem.updated_at ? new Date(mem.updated_at).toLocaleString() : '-'}</span>
          <span>Accessed: ${mem.access_count || 0} times</span>
          <span>Peer: ${esc(node)}</span>
          <span>Session: ${esc(session)}</span>
        </div>
        <div style="margin-top:8px;font-family:var(--font-mono);font-size:10px;color:var(--text-muted)">ID: ${esc(mem.id)}</div>
        <div style="margin-top:12px;display:flex;gap:8px">
          <button class="btn-search btn-md" id="mem-promote-btn">Create Manifest from Memory</button>
          <button class="btn-dismiss btn-md" onclick="OL.archiveMemory('${esc(mem.id)}')">Archive</button>
        </div>
      </div>
    `;

    OL.onView(detail.querySelector('#mem-promote-btn'), 'click', () => {
      OL.promoteToManifest(
        tierContent.l0,
        tierContent.l1,
        tierContent.l2
      );
    });

    detail.querySelectorAll('.tier-tab').forEach(tab => {
      var handler = function() {
        detail.querySelectorAll('.tier-tab').forEach(t => { t.classList.remove('active'); t.setAttribute('aria-selected', 'false'); });
        tab.classList.add('active');
        tab.setAttribute('aria-selected', 'true');
        document.getElementById('memory-peer-content-text').textContent = tierContent[tab.dataset.tier];
      };
      OL.onView(tab, 'click', handler);
      OL.onView(tab, 'keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handler(); }
      });
    });
  }

  OL.archiveMemory = async function(id) {
    // Memories don't have a status field — soft delete via API
    await fetch('/api/memories/' + id, {method: 'DELETE'});
    document.getElementById('memory-peer-detail').innerHTML = '<div class="empty-state">Archived</div>';
    OL.loadMemoryPeerTree();
  };

})(window.OL);
