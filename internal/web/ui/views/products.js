(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  OL.loadProducts = async function() {
    var el = document.getElementById('products-list');
    try {
      var peerGroups = await fetchJSON('/api/products/by-peer');

      OL.renderTree(el, peerGroups, {
        prefix: 'prod',
        emptyMessage: 'No products yet. Create one to group your manifests.',
        levels: [
          {
            label: function(pg) { return esc(pg.peer_id); },
            count: function(pg) { return pg.count; },
            children: function(pg) { return pg.products; },
          },
          {
            label: function(p) { return esc(p.title); },
            extra: function(p) {
              return '<span class="session-uuid">' + esc(p.marker) + '</span>' +
                (p.total_manifests > 0 ? '<span class="count">' + p.total_manifests + '</span>' : '');
            },
            dotColor: function(p) {
              return p.status === 'open' ? 'green' : p.status === 'closed' ? 'red' : p.status === 'archive' ? 'red' : 'yellow';
            },
            expanded: false,
            nodeAttrs: function(p) { return 'data-product-id="' + esc(p.id) + '"'; },
            renderContent: function(p) {
              var metaParts = [];
              if (p.total_tasks > 0) metaParts.push(p.total_tasks + ' tasks');
              if (p.total_turns > 0) metaParts.push(p.total_turns + ' turns');
              if (p.total_cost > 0) metaParts.push('$' + p.total_cost.toFixed(2));
              return '<div class="prod-manifests-placeholder" data-prod-manifests-for="' + esc(p.id) + '" style="margin-left:24px">' +
                '<div style="padding:8px 12px;color:var(--text-muted);font-size:12px">' +
                  (metaParts.length ? metaParts.join(' &middot; ') : '') +
                  (p.total_manifests > 0 ? '<div style="margin-top:4px;font-style:italic">Loading manifests...</div>' : '<div style="margin-top:4px;font-style:italic">No manifests linked</div>') +
                '</div>' +
              '</div>';
            },
            onClick: function(node, childrenEl, nowExpanded, p) {
              OL.loadProductDetail(p.id);
              if (nowExpanded) {
                var placeholder = childrenEl.querySelector('.prod-manifests-placeholder');
                if (placeholder) loadProductManifests(p.id, placeholder);
              }
            },
          }
        ],
        afterRender: function(container) {
          container.insertAdjacentHTML('afterbegin', '<div style="padding:8px 0;margin-bottom:8px"><button class="btn-search" id="btn-new-product" style="font-size:12px;padding:6px 16px">+ New Product</button></div>');
          OL.onView(container.querySelector('#btn-new-product'), 'click', function() { window._createProduct(); });
        },
      });

      // Handle empty case — still need new product button
      if (!peerGroups || !peerGroups.length) {
        el.insertAdjacentHTML('afterbegin', '<div style="padding:8px 0;margin-bottom:8px"><button class="btn-search" id="btn-new-product" style="font-size:12px;padding:6px 16px">+ New Product</button></div>');
        OL.onView(el.querySelector('#btn-new-product'), 'click', function() { window._createProduct(); });
      }
    } catch (e) {
      console.error('Load products failed:', e);
    }
  };

  async function loadProductManifests(productId, container) {
    try {
      const manifests = await fetchJSON('/api/products/' + productId + '/manifests');
      if (!manifests || !manifests.length) {
        container.innerHTML = '<div style="padding:8px 12px;color:var(--text-muted);font-size:12px;font-style:italic">No manifests linked</div>';
        return;
      }
      let html = '';
      for (const m of manifests) {
        const statusClass = m.status === 'open' ? 'confirmed' : m.status === 'closed' ? 'flagged' : 'dismissed';
        html += `<div class="amnesia-item ${statusClass}" style="cursor:pointer" onclick="OL.switchView('manifests');setTimeout(()=>window._loadManifest&&window._loadManifest('${esc(m.id)}'),300)">
          <div class="amnesia-header">
            <span class="amnesia-status-label">${esc(m.status)}</span>
            <span class="session-uuid">${esc(m.marker)}</span>
            <span style="color:var(--text-muted);font-size:11px;margin-left:auto">${formatTime(m.updated_at)}</span>
          </div>
          <div class="amnesia-rule">${esc(m.title)}</div>
        </div>`;
      }
      container.innerHTML = html;
    } catch (e) {
      container.innerHTML = '<div style="padding:8px 12px;color:var(--red);font-size:12px">Failed to load manifests</div>';
    }
  }

  // Create product dialog
  window._createProduct = function() {
    const titleEl = document.getElementById('product-detail-title');
    const bodyEl = document.getElementById('product-detail');
    titleEl.textContent = 'New Product';
    bodyEl.innerHTML = `
      <div style="max-width:500px">
        <div style="margin-bottom:12px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Title *</label>
          <input id="new-product-title" class="conv-search" style="width:100%;padding:8px 12px;font-size:14px" placeholder="Product name" autofocus>
        </div>
        <div style="margin-bottom:12px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Description</label>
          <textarea id="new-product-desc" class="conv-search" style="width:100%;min-height:80px;padding:8px 12px;font-size:13px;resize:vertical" placeholder="What is this product/project about?"></textarea>
        </div>
        <div style="margin-bottom:16px">
          <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Tags (comma-separated)</label>
          <input id="new-product-tags" class="conv-search" style="width:100%;padding:8px 12px;font-size:13px" placeholder="e.g. openloom, backend">
        </div>
        <div style="display:flex;gap:8px">
          <button class="btn-search" id="btn-save-product" style="padding:6px 20px;font-size:13px">Create Product</button>
          <button class="btn-dismiss" onclick="OL.loadProducts()" style="padding:6px 16px;font-size:13px">Cancel</button>
        </div>
      </div>`;
    OL.onView(document.getElementById('btn-save-product'), 'click', async () => {
      const title = document.getElementById('new-product-title').value.trim();
      if (!title) { alert('Title is required'); return; }
      const desc = document.getElementById('new-product-desc').value.trim();
      const tags = document.getElementById('new-product-tags').value.split(',').map(t => t.trim()).filter(Boolean);
      try {
        const p = await fetchJSON('/api/products', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({title, description: desc, tags}) });
        OL.loadProducts();
        if (p && p.id) setTimeout(() => OL.loadProductDetail(p.id), 300);
      } catch(e) { alert('Error: ' + e.message); }
    });
    document.getElementById('new-product-title').focus();
  };

  OL.loadProductDetail = async function(id) {
    document.querySelectorAll('[data-product-id]').forEach(i => i.classList.remove('active'));
    const active = document.querySelector(`[data-product-id="${id}"]`);
    if (active) active.classList.add('active');

    try {
      const p = await fetchJSON('/api/products/' + id);
      if (!p) return;

      const titleEl = document.getElementById('product-detail-title');
      const bodyEl = document.getElementById('product-detail');

      titleEl.textContent = p.title;

      const tags = (p.tags || []).map(t => `<span class="badge tag">${esc(t)}</span>`).join(' ');
      const statusClass = p.status === 'open' ? 'scope' : p.status === 'closed' ? 'type' : p.status === 'draft' ? '' : p.status === 'archive' ? 'type' : '';

      // Fetch linked manifests
      let manifestsHtml = '';
      try {
        const manifests = await fetchJSON('/api/products/' + p.id + '/manifests');
        if (manifests && manifests.length) {
          const manifestRows = manifests.map(m => {
            const mStatusClass = m.status === 'open' ? 'scope' : m.status === 'closed' ? 'type' : m.status === 'archive' ? 'type' : '';
            const mCostParts = [];
            if (m.total_tasks > 0) mCostParts.push(`${m.total_tasks} tasks`);
            if (m.total_turns > 0) mCostParts.push(`${m.total_turns} turns`);
            if (m.total_cost > 0) mCostParts.push(`<span style="color:var(--green)">$${m.total_cost.toFixed(2)}</span>`);
            return `<div class="manifest-item" style="border-bottom:1px solid var(--border);padding:10px 12px;display:flex;align-items:center">
              <div style="flex:1;cursor:pointer" onclick="OL.switchView('manifests');setTimeout(()=>window._loadManifest('${esc(m.id)}'),300)">
                <div style="display:flex;align-items:center;gap:8px">
                  <span class="session-uuid" style="font-size:11px">${esc(m.marker)}</span>
                  <span class="badge ${mStatusClass}" style="font-size:10px">${esc(m.status)}</span>
                  ${mCostParts.length ? mCostParts.join('<span style="opacity:0.3;font-size:10px"> | </span>') : ''}
                </div>
                <div style="font-size:13px;color:var(--text-primary);margin-top:4px">${esc(m.title)}</div>
              </div>
              <button class="btn-dismiss" style="font-size:10px;padding:2px 8px;flex-shrink:0" onclick="event.stopPropagation();window._unlinkManifestFromProduct('${esc(m.id)}','${esc(p.id)}')" title="Remove from product">&#x2715;</button>
            </div>`;
          }).join('');
          manifestsHtml = `<div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="font-size:13px;color:var(--text-primary);margin-bottom:8px;font-weight:600">Linked Manifests <span style="font-weight:400;font-size:12px;color:var(--text-muted)">(${manifests.length})</span></div>
            <div style="border:1px solid var(--border);border-radius:4px;overflow:hidden">${manifestRows}</div>
          </div>`;
        } else {
          manifestsHtml = `<div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="font-size:13px;color:var(--text-primary);margin-bottom:8px;font-weight:600">Linked Manifests</div>
            <div style="font-size:12px;color:var(--text-muted);padding:12px;border:1px dashed var(--border);border-radius:4px;text-align:center">No manifests linked yet</div>
          </div>`;
        }
      } catch(e) {}

      // Fetch linked ideas
      let ideasHtml = '';
      try {
        const ideas = await fetchJSON('/api/products/' + p.id + '/ideas');
        if (ideas && ideas.length) {
          const ideaRows = ideas.map(i => {
            const prioColor = i.priority === 'critical' ? 'var(--red)' : i.priority === 'high' ? 'var(--yellow)' : 'var(--text-muted)';
            return `<div class="manifest-item" style="border-bottom:1px solid var(--border);padding:8px 12px;display:flex;align-items:center">
              <div style="flex:1">
                <div style="display:flex;align-items:center;gap:8px">
                  <span class="session-uuid" style="font-size:11px">${esc(i.marker)}</span>
                  <span class="badge" style="font-size:10px">${esc(i.status)}</span>
                  <span style="font-size:10px;color:${prioColor}">${esc(i.priority)}</span>
                </div>
                <div style="font-size:13px;color:var(--text-primary);margin-top:2px">${esc(i.title)}</div>
              </div>
              <button class="btn-dismiss" style="font-size:10px;padding:2px 8px;flex-shrink:0" onclick="event.stopPropagation();window._unlinkIdeaFromProduct('${esc(i.id)}','${esc(p.id)}')" title="Remove from product">&#x2715;</button>
            </div>`;
          }).join('');
          ideasHtml = `<div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span style="font-size:13px;color:var(--text-primary);font-weight:600">Linked Ideas <span style="font-weight:400;font-size:12px;color:var(--text-muted)">(${ideas.length})</span></span>
              <button class="btn-search" style="font-size:11px;padding:2px 10px" onclick="window._linkIdeaToProduct('${esc(p.id)}')">+ Link Idea</button>
            </div>
            <div style="border:1px solid var(--border);border-radius:4px;overflow:hidden">${ideaRows}</div>
          </div>`;
        } else {
          ideasHtml = `<div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span style="font-size:13px;color:var(--text-primary);font-weight:600">Linked Ideas</span>
              <button class="btn-search" style="font-size:11px;padding:2px 10px" onclick="window._linkIdeaToProduct('${esc(p.id)}')">+ Link Idea</button>
            </div>
            <div style="font-size:12px;color:var(--text-muted);padding:12px;border:1px dashed var(--border);border-radius:4px;text-align:center">No ideas linked yet</div>
          </div>`;
        }
      } catch(e) {}

      bodyEl.innerHTML = `
        <div>
          <!-- BREADCRUMB -->
          <div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;font-family:var(--font-mono)">
            <span style="cursor:pointer;color:var(--accent)" onclick="OL.switchView('products')">${esc(p.source_node ? p.source_node.substring(0,12) : 'node')}</span>
            <span style="opacity:0.4"> &rarr; </span>
            <span style="color:var(--text-primary)">${esc(p.marker)} ${esc(p.title)}</span>
          </div>
          <!-- METADATA BAR -->
          <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:16px">
            <span class="session-uuid" style="font-size:14px">${esc(p.marker)}</span>
            <span class="badge ${statusClass}">${esc(p.status)}</span>
            ${tags}
            <button class="btn-copy" onclick="window._copy('get product ${esc(p.marker)}')" title="Copy ref">&#x2398;</button>
          </div>
          <!-- METRICS BAR -->
          <div style="display:flex;gap:12px;font-size:12px;color:var(--text-muted);margin-bottom:12px;align-items:center;flex-wrap:wrap;padding:8px 12px;background:var(--bg-secondary);border:1px solid var(--border);border-radius:6px;font-family:var(--font-mono)">
            <span>Manifests: <strong style="color:var(--text-primary)">${p.total_manifests || 0}</strong></span>
            <span>Tasks: <strong style="color:var(--text-primary)">${p.total_tasks || 0}</strong></span>
            <span>Turns: <strong style="color:var(--text-primary)">${p.total_turns || 0}</strong></span>
            <span>Cost: <strong style="color:var(--green)">$${(p.total_cost || 0).toFixed(2)}</strong></span>
            <span style="opacity:0.3">|</span>
            <span>Created: ${new Date(p.created_at).toLocaleString()}</span>
            <span>Updated: ${new Date(p.updated_at).toLocaleString()}</span>
          </div>
          <!-- CONTROLS -->
          <div style="display:flex;gap:8px;margin-bottom:12px;flex-wrap:wrap">
            <button class="btn-search" style="font-size:11px;padding:4px 12px" onclick="window._editProduct('${esc(p.id)}')">&#9998; Edit</button>
            <button class="btn-search" style="font-size:11px;padding:4px 12px;background:var(--bg-input)" onclick="window._linkManifestToProduct('${esc(p.id)}')">+ Link Manifest</button>
            ${['draft','open','closed','archive'].map(s => {
              const active = p.status === s;
              const color = s === 'open' ? 'var(--green)' : s === 'closed' ? 'var(--text-muted)' : s === 'archive' ? 'var(--red)' : 'var(--yellow)';
              return '<button class="product-status-btn" data-status="' + s + '" style="padding:4px 12px;font-size:11px;font-weight:600;text-transform:uppercase;border-radius:4px;cursor:pointer;border:1px solid ' + color + ';background:' + (active ? color : 'transparent') + ';color:' + (active ? 'var(--bg-primary)' : color) + ';opacity:' + (active ? '1' : '0.7') + '" onclick="window._updateProductStatus(\'' + esc(p.id) + '\',\'' + s + '\')">' + s + '</button>';
            }).join('')}
          </div>
          ${p.description ? `<div style="font-size:13px;color:var(--text-secondary);line-height:1.6;margin-bottom:12px;white-space:pre-wrap">${esc(p.description)}</div>` : ''}
          <!-- DIAGRAM BUTTON -->
          <div style="margin-bottom:12px">
            <button class="btn-search" style="font-size:12px;padding:6px 16px" onclick="window._showProductDiagram('${esc(p.id)}','${esc(p.title)}')">&#x25C8; Product DAG</button>
          </div>
          ${manifestsHtml}
          ${ideasHtml}
        </div>`;

    } catch (e) {
      console.error('Load product detail failed:', e);
    }
  };

  window._loadProduct = OL.loadProductDetail;

  // --- Cytoscape Directed Acyclic Graph Diagram (Full Page) ---
  window._showProductDiagram = function(productId, productTitle) {
    // Create full-page overlay
    let overlay = document.getElementById('product-diagram-overlay');
    if (overlay) overlay.remove();
    overlay = document.createElement('div');
    overlay.id = 'product-diagram-overlay';
    overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;z-index:1000;background:var(--bg-primary);display:flex;flex-direction:column';
    overlay.innerHTML = `
      <div style="display:flex;align-items:center;gap:12px;padding:12px 20px;border-bottom:1px solid var(--border);background:var(--bg-secondary);flex-shrink:0">
        <button id="diagram-back-btn" style="padding:6px 14px;font-size:12px;font-weight:600;border:1px solid var(--border);border-radius:4px;cursor:pointer;background:var(--bg-input);color:var(--text-primary)">&#x2190; Back</button>
        <span style="font-size:15px;font-weight:600;color:var(--text-primary)">${esc(productTitle)}</span>
        <span style="font-size:12px;color:var(--text-muted)">Directed Acyclic Graph</span>
        <span style="margin-left:auto;display:flex;align-items:center;gap:16px;font-size:11px;color:var(--text-muted)">
          <span style="display:flex;align-items:center;gap:4px"><span style="display:inline-block;width:20px;height:0;border-top:2px solid rgba(255,255,255,0.15)"></span>hierarchy</span>
          <span style="display:flex;align-items:center;gap:4px"><span style="display:inline-block;width:20px;height:0;border-top:2px dashed #f59e0b"></span>depends on</span>
          <span>Scroll to zoom &middot; Drag to pan &middot; Click node to drill down</span>
        </span>
      </div>
      <div id="product-cytoscape" style="flex:1;width:100%"></div>
    `;
    document.body.appendChild(overlay);
    OL.onView(document.getElementById('diagram-back-btn'), 'click', () => overlay.remove());
    // ESC to close
    const escHandler = (e) => { if (e.key === 'Escape') { overlay.remove(); document.removeEventListener('keydown', escHandler); } };
    OL.onView(document, 'keydown', escHandler);
    renderProductDiagram(productId);
  };

  async function renderProductDiagram(productId) {
    const container = document.getElementById('product-cytoscape');
    if (!container || typeof cytoscape === 'undefined') return;
    container.innerHTML = '<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--text-muted)">Loading...</div>';

    try {
      const data = await fetchJSON('/api/products/' + productId + '/hierarchy');
      if (!data) return;

      const elements = [];
      const statusColor = (type, status) => {
        if (type === 'task') {
          if (status === 'completed') return '#00d97e';
          if (status === 'running') return '#f5c542';
          if (status === 'failed') return '#e63757';
          if (status === 'scheduled' || status === 'waiting') return '#f5c542';
          return '#71717a';
        }
        // product + manifest
        if (status === 'closed' || status === 'archive') return '#00d97e';
        if (status === 'open') return '#3b82f6';
        if (status === 'draft') return '#f5c542';
        return '#71717a';
      };

      const nodeShape = (type) => type === 'product' ? 'round-rectangle' : type === 'manifest' ? 'round-rectangle' : 'ellipse';
      const nodeSize = (type) => type === 'product' ? 60 : type === 'manifest' ? 45 : 30;

      const nodeLabel = (type) => {
        return type === 'product' ? 'Product' : type === 'manifest' ? 'Manifest' : 'Task';
      };

      // Add product node
      elements.push({ data: {
        id: data.id, label: nodeLabel(data.type), title: data.title, type: data.type, status: data.status,
        marker: data.marker, color: statusColor(data.type, data.status),
        nodeSize: nodeSize(data.type), meta: JSON.stringify(data.meta || {})
      }});

      // Add manifest + task nodes
      for (const m of (data.children || [])) {
        elements.push({ data: {
          id: m.id, label: nodeLabel(m.type), title: m.title, type: m.type, status: m.status,
          marker: m.marker, color: statusColor(m.type, m.status),
          nodeSize: nodeSize(m.type), meta: JSON.stringify(m.meta || {})
        }});
        elements.push({ data: { source: data.id, target: m.id } });

        for (const t of (m.children || [])) {
          elements.push({ data: {
            id: t.id, label: nodeLabel(t.type), title: t.title, type: t.type, status: t.status,
            marker: t.marker, color: statusColor(t.type, t.status),
            nodeSize: nodeSize(t.type), meta: JSON.stringify(t.meta || {}),
            depends_on: t.depends_on || ''
          }});
          // Only connect task to manifest if it has no dependency -- chained tasks connect to their predecessor instead
          if (!t.depends_on) {
            elements.push({ data: { source: m.id, target: t.id } });
          }
        }
      }

      // Add dependency edges between tasks (dashed, orange)
      for (const el of [...elements]) {
        const d = el.data;
        if (d && d.type === 'task' && d.depends_on) {
          // Edge from dependency TO dependent (arrow shows execution order)
          const depExists = elements.some(e => e.data && e.data.id === d.depends_on);
          if (depExists) {
            elements.push({ data: { source: d.depends_on, target: d.id, edgeType: 'dependency' } });
          }
        }
      }

      container.innerHTML = '';
      const cy = cytoscape({
        container: container,
        elements: elements,
        layout: {
          name: 'dagre',
          rankDir: 'TB',
          spacingFactor: 1.4,
          nodeSep: 30,
          rankSep: 60,
        },
        style: [
          {
            selector: 'node',
            style: {
              'label': 'data(label)',
              'text-wrap': 'wrap',
              'text-max-width': '120px',
              'font-size': '10px',
              'text-valign': 'center',
              'text-halign': 'center',
              'color': '#e4e4e7',
              'background-color': 'data(color)',
              'border-width': 2,
              'border-color': 'data(color)',
              'width': 'data(nodeSize)',
              'height': 'data(nodeSize)',
              'shape': 'round-rectangle',
              'text-outline-color': '#0a0a0f',
              'text-outline-width': 2,
            }
          },
          {
            selector: 'node[type="task"]',
            style: { 'shape': 'ellipse', 'font-size': '8px', 'text-max-width': '90px' }
          },
          {
            selector: 'node[type="product"]',
            style: { 'font-size': '12px', 'font-weight': 'bold', 'text-max-width': '160px' }
          },
          {
            selector: 'edge',
            style: {
              'width': 1.5,
              'line-color': 'rgba(255,255,255,0.15)',
              'target-arrow-color': 'rgba(255,255,255,0.15)',
              'target-arrow-shape': 'triangle',
              'curve-style': 'bezier',
              'arrow-scale': 0.8,
            }
          },
          {
            selector: 'edge[edgeType="dependency"]',
            style: {
              'width': 2,
              'line-color': '#f59e0b',
              'line-style': 'dashed',
              'line-dash-pattern': [6, 3],
              'target-arrow-color': '#f59e0b',
              'target-arrow-shape': 'triangle',
              'curve-style': 'bezier',
              'arrow-scale': 0.9,
            }
          },
          {
            selector: 'node:active, node:selected',
            style: { 'border-width': 3, 'border-color': '#3b82f6', 'overlay-opacity': 0 }
          },
        ],
        minZoom: 0.3,
        maxZoom: 3,
        wheelSensitivity: 0.3,
      });

      // Tooltip
      const tooltip = document.createElement('div');
      tooltip.style.cssText = 'position:absolute;display:none;background:rgba(10,10,15,0.95);border:1px solid rgba(255,255,255,0.15);border-radius:6px;padding:8px 12px;font-size:11px;color:#e4e4e7;pointer-events:none;z-index:100;max-width:250px;font-family:var(--font-mono)';
      container.style.position = 'relative';
      container.appendChild(tooltip);

      cy.on('mouseover', 'node', (e) => {
        const node = e.target;
        const d = node.data();
        const meta = JSON.parse(d.meta || '{}');
        let html = `<div style="font-weight:600;margin-bottom:4px">${esc(d.title)}</div>`;
        html += `<div><span style="color:var(--text-muted)">Status:</span> <span style="color:${d.color}">${d.status}</span></div>`;
        html += `<div style="color:var(--text-muted);font-size:10px;margin-bottom:4px">${d.marker}</div>`;
        if (d.type === 'product') {
          html += `<div>${meta.total_manifests || 0} manifests</div>`;
          html += `<div>${meta.total_tasks || 0} tasks</div>`;
        } else if (d.type === 'manifest') {
          html += `<div>${meta.total_tasks || 0} tasks</div>`;
        } else {
          html += `<div>${meta.turns || 0} turns</div>`;
          if (meta.run_count > 0) html += `<div>${meta.run_count} runs</div>`;
          if (d.depends_on) {
            const depNode = cy.getElementById(d.depends_on);
            const depTitle = depNode.length ? depNode.data('title') : d.depends_on.slice(0, 8);
            html += `<div style="color:#f59e0b">depends on: ${esc(depTitle)}</div>`;
          }
        }
        if (meta.total_cost > 0 || meta.cost_usd > 0) html += `<div><span style="color:var(--green)">$${(meta.total_cost || meta.cost_usd || 0).toFixed(2)}</span></div>`;
        tooltip.innerHTML = html;
        tooltip.style.display = '';
        const pos = node.renderedPosition();
        tooltip.style.left = (pos.x + 20) + 'px';
        tooltip.style.top = (pos.y - 20) + 'px';
      });
      cy.on('mouseout', 'node', () => { tooltip.style.display = 'none'; });

      // Click to drill down
      cy.on('tap', 'node', (e) => {
        const d = e.target.data();
        const overlay = document.getElementById('product-diagram-overlay');
        if (overlay) overlay.remove();
        if (d.type === 'product') {
          OL.switchView('products');
          setTimeout(() => OL.loadProductDetail(d.id), 300);
        } else if (d.type === 'manifest') {
          OL.switchView('manifests');
          setTimeout(() => window._loadManifest(d.id), 300);
        } else if (d.type === 'task') {
          OL.switchView('tasks');
          setTimeout(() => OL.loadTaskDetail(d.id), 300);
        }
      });

    } catch (e) {
      container.innerHTML = '<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--red)">Failed to load diagram</div>';
      console.error('Diagram error:', e);
    }
  }

  // Product CRUD actions
  window._editProduct = function(id) {
    const titleEl = document.getElementById('product-detail-title');
    const bodyEl = document.getElementById('product-detail');
    fetchJSON('/api/products/' + id).then(p => {
      if (!p) return;
      titleEl.textContent = 'Edit Product';
      bodyEl.innerHTML = `
        <div style="max-width:500px">
          <div style="margin-bottom:12px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Title</label>
            <input id="edit-product-title" class="conv-search" style="width:100%;padding:8px 12px;font-size:14px" value="${esc(p.title)}">
          </div>
          <div style="margin-bottom:12px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Description</label>
            <textarea id="edit-product-desc" class="conv-search" style="width:100%;min-height:80px;padding:8px 12px;font-size:13px;resize:vertical">${esc(p.description)}</textarea>
          </div>
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px">Tags (comma-separated)</label>
            <input id="edit-product-tags" class="conv-search" style="width:100%;padding:8px 12px;font-size:13px" value="${esc((p.tags||[]).join(', '))}">
          </div>
          <div style="display:flex;gap:8px">
            <button class="btn-search" id="btn-update-product" style="padding:6px 20px;font-size:13px">Save</button>
            <button class="btn-dismiss" onclick="OL.loadProductDetail('${esc(p.id)}')" style="padding:6px 16px;font-size:13px">Cancel</button>
          </div>
        </div>`;
      OL.onView(document.getElementById('btn-update-product'), 'click', async () => {
        const title = document.getElementById('edit-product-title').value.trim();
        if (!title) { alert('Title is required'); return; }
        const description = document.getElementById('edit-product-desc').value.trim();
        const tags = document.getElementById('edit-product-tags').value.split(',').map(t => t.trim()).filter(Boolean);
        await fetchJSON('/api/products/' + p.id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({title, description, tags}) });
        OL.loadProducts();
        OL.loadProductDetail(p.id);
      });
    });
  };

  window._updateProductStatus = async function(id, status) {
    await fetchJSON('/api/products/' + id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({status}) });
    OL.loadProducts();
    OL.loadProductDetail(id);
  };

  // _deleteProduct removed -- use status toggle to archive instead

  window._linkManifestToProduct = async function(productId) {
    // Fetch all manifests not yet linked to this product
    const allManifests = await fetchJSON('/api/manifests');
    const unlinked = (allManifests || []).filter(m => m.project_id !== productId);
    if (!unlinked.length) { alert('No manifests available to link'); return; }

    const bodyEl = document.getElementById('product-detail');
    const origHtml = bodyEl.innerHTML;
    let listHtml = unlinked.map(m =>
      `<div class="manifest-item clickable" data-link-mid="${esc(m.id)}" style="padding:8px 12px;border-bottom:1px solid var(--border)">
        <span class="session-uuid" style="font-size:11px">${esc(m.marker)}</span>
        <span class="badge" style="font-size:10px">${esc(m.status)}</span>
        <span style="font-size:12px">${esc(m.title)}</span>
      </div>`
    ).join('');

    bodyEl.innerHTML = `
      <div>
        <div style="font-size:14px;font-weight:600;margin-bottom:12px">Link Manifest to Product</div>
        <div style="margin-bottom:12px;font-size:12px;color:var(--text-muted)">Click a manifest to link it:</div>
        <div style="border:1px solid var(--border);border-radius:4px;max-height:400px;overflow-y:auto">${listHtml}</div>
        <button class="btn-dismiss" style="margin-top:12px;padding:6px 16px;font-size:13px" onclick="OL.loadProductDetail('${esc(productId)}')">Cancel</button>
      </div>`;

    bodyEl.querySelectorAll('[data-link-mid]').forEach(item => {
      OL.onView(item, 'click', async () => {
        const mid = item.dataset.linkMid;
        await fetchJSON('/api/manifests/' + mid, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: productId}) });
        OL.loadProducts();
        OL.loadProductDetail(productId);
      });
    });
  };

  window._unlinkManifestFromProduct = async function(manifestId, productId) {
    await fetchJSON('/api/manifests/' + manifestId, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: ''}) });
    OL.loadProducts();
    OL.loadProductDetail(productId);
  };

  window._linkIdeaToProduct = async function(productId) {
    const allIdeas = await fetchJSON('/api/ideas');
    const unlinked = (allIdeas || []).filter(i => i.project_id !== productId);
    if (!unlinked.length) { alert('No ideas available to link'); return; }

    const bodyEl = document.getElementById('product-detail');
    let listHtml = unlinked.map(i =>
      `<div class="manifest-item clickable" data-link-iid="${esc(i.id)}" style="padding:8px 12px;border-bottom:1px solid var(--border)">
        <span class="session-uuid" style="font-size:11px">${esc(i.marker)}</span>
        <span class="badge" style="font-size:10px">${esc(i.status)}</span>
        <span style="font-size:10px;color:var(--text-muted)">${esc(i.priority)}</span>
        <span style="font-size:12px">${esc(i.title)}</span>
      </div>`
    ).join('');

    bodyEl.innerHTML = `
      <div>
        <div style="font-size:14px;font-weight:600;margin-bottom:12px">Link Idea to Product</div>
        <div style="margin-bottom:12px;font-size:12px;color:var(--text-muted)">Click an idea to link it:</div>
        <div style="border:1px solid var(--border);border-radius:4px;max-height:400px;overflow-y:auto">${listHtml}</div>
        <button class="btn-dismiss" style="margin-top:12px;padding:6px 16px;font-size:13px" onclick="OL.loadProductDetail('${esc(productId)}')">Cancel</button>
      </div>`;

    bodyEl.querySelectorAll('[data-link-iid]').forEach(item => {
      OL.onView(item, 'click', async () => {
        const iid = item.dataset.linkIid;
        const idea = (allIdeas || []).find(i => i.id === iid);
        if (idea) {
          await fetchJSON('/api/ideas/' + iid, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: productId, title: idea.title, description: idea.description, status: idea.status, priority: idea.priority}) });
        }
        OL.loadProducts();
        OL.loadProductDetail(productId);
      });
    });
  };

  window._unlinkIdeaFromProduct = async function(ideaId, productId) {
    const idea = await fetchJSON('/api/ideas/' + ideaId);
    if (idea) {
      await fetchJSON('/api/ideas/' + ideaId, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: '', title: idea.title, description: idea.description, status: idea.status, priority: idea.priority}) });
    }
    OL.loadProducts();
    OL.loadProductDetail(productId);
  };

})(window.OL);
