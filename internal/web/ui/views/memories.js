(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  OL.loadMemoryPeerTree = async function() {
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
            '<button class="btn-copy-sm" onclick="event.stopPropagation();window._copy(\'recall memory ' + esc(m.marker) + '\')" title="Copy ref">&#x2398;</button>' +
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
          <button class="btn-copy" onclick="window._copy('recall memory ${marker}')" title="Copy reference">&#x2398;</button>
        </div>
        <div class="memory-meta">
          <span class="badge type">${esc(mem.type || 'insight')}</span>
          <span class="badge scope">${esc(mem.scope || 'project')}</span>
          ${mem.project ? `<span class="badge">${esc(mem.project)}</span>` : ''}
          ${mem.domain ? `<span class="badge">${esc(mem.domain)}</span>` : ''}
          <span class="badge" style="color:var(--green)">${esc(session)}</span>
          <span class="badge" style="color:var(--accent)">${esc(node)}</span>
        </div>
        <div class="tier-tabs">
          <div class="tier-tab" data-tier="l0">L0 — One-liner</div>
          <div class="tier-tab" data-tier="l1">L1 — Summary</div>
          <div class="tier-tab active" data-tier="l2">L2 — Full</div>
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
          <button class="btn-search" id="mem-promote-btn" style="font-size:12px;padding:6px 14px">Create Manifest from Memory</button>
          <button class="btn-dismiss" onclick="window._archiveMemory('${esc(mem.id)}')" style="font-size:12px;padding:6px 14px">Archive</button>
        </div>
      </div>
    `;

    OL.onView(detail.querySelector('#mem-promote-btn'), 'click', () => {
      window._promoteToManifest(
        tierContent.l0,
        tierContent.l1,
        tierContent.l2
      );
    });

    detail.querySelectorAll('.tier-tab').forEach(tab => {
      OL.onView(tab, 'click', () => {
        detail.querySelectorAll('.tier-tab').forEach(t => t.classList.remove('active'));
        tab.classList.add('active');
        document.getElementById('memory-peer-content-text').textContent = tierContent[tab.dataset.tier];
      });
    });
  }

  window._archiveMemory = async function(id) {
    // Memories don't have a status field — soft delete via API
    await fetch('/api/memories/' + id, {method: 'DELETE'});
    document.getElementById('memory-peer-detail').innerHTML = '<div class="empty-state">Archived</div>';
    OL.loadMemoryPeerTree();
  };

})(window.OL);
