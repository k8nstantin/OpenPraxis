// Product DAG overlay — extracted from views/products.js so the layout
// code lives in one file with a single responsibility. This module is
// loaded by index.html BEFORE views/products.js because products.js calls
// OL.showProductDiagram from its list-row buttons.
//
// Layout strategy: cytoscape + cytoscape-dagre. No hand-rolled positions,
// no manual topo sort, no DFS ordering. Dagre takes the edge set and
// computes the layout. Edges are driven from real depends_on values and
// from ownership (product → manifest, manifest → task). Any DAG shape
// renders correctly without layout-specific code.
//
// Libraries are pinned + served from /vendor/* (see internal/web/ui/vendor/,
// loaded in index.html). If those assets fail to load, this module's
// renderProductDiagram is a no-op that shows an error in the overlay.
(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc;

  OL.showProductDiagram = function(productId, productTitle) {
    var overlay = document.getElementById('product-diagram-overlay');
    if (overlay) overlay.remove();
    overlay = document.createElement('div');
    overlay.id = 'product-diagram-overlay';
    overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;z-index:1000;background:var(--bg-primary);display:flex;flex-direction:column';
    overlay.innerHTML =
      '<div style="display:flex;align-items:center;gap:12px;padding:12px 20px;border-bottom:1px solid var(--border);background:var(--bg-secondary);flex-shrink:0">' +
        '<button id="diagram-back-btn" style="padding:6px 14px;font-size:12px;font-weight:600;border:1px solid var(--border);border-radius:4px;cursor:pointer;background:var(--bg-input);color:var(--text-primary)">&#x2190; Back</button>' +
        '<span style="font-size:15px;font-weight:600;color:var(--text-primary)">' + esc(productTitle) + '</span>' +
        '<span style="font-size:12px;color:var(--text-muted)">Directed Acyclic Graph</span>' +
        '<span style="margin-left:auto;display:flex;align-items:center;gap:16px;font-size:11px;color:var(--text-muted)">' +
          '<span style="display:flex;align-items:center;gap:4px"><span style="display:inline-block;width:20px;height:0;border-top:2px solid rgba(255,255,255,0.15)"></span>hierarchy</span>' +
          '<span style="display:flex;align-items:center;gap:4px"><span style="display:inline-block;width:20px;height:0;border-top:2.5px solid #3b82f6"></span>manifest dep</span>' +
          '<span style="display:flex;align-items:center;gap:4px"><span style="display:inline-block;width:20px;height:0;border-top:2px dashed #f59e0b"></span>task dep</span>' +
          '<span>Scroll to zoom &middot; Drag to pan &middot; Click node to drill down</span>' +
        '</span>' +
      '</div>' +
      '<div id="product-cytoscape" style="flex:1;width:100%"></div>';
    document.body.appendChild(overlay);
    OL.onView(document.getElementById('diagram-back-btn'), 'click', function() { overlay.remove(); });
    var escHandler = function(e) { if (e.key === 'Escape') { overlay.remove(); document.removeEventListener('keydown', escHandler); } };
    OL.onView(document, 'keydown', escHandler);
    renderProductDiagram(productId);
  };

  // buildDagElements is exported on OL so tests / inspection tools can
  // verify the edge set independently of cytoscape. Given a product
  // hierarchy payload (shape matches /api/products/{id}/hierarchy), it
  // returns the { nodes, edges } that get handed to cytoscape. Edges are
  // what dagre uses to rank the layout — if the edges are correct, the
  // layout is correct, regardless of DAG shape.
  OL.buildDagElements = function(data) {
    var elements = [];
    if (!data) return elements;
    var manifests = data.children || [];

    function shortLabel(title) {
      return title.replace(/^QA\s+/, '').replace(/^OpenPraxis\s+/, '').replace(/\s*—\s*.+$/, '');
    }

    // Product node
    elements.push({ data: {
      id: data.id, label: data.title, title: data.title, type: 'product',
      status: data.status, marker: data.marker,
      meta: JSON.stringify(data.meta || {})
    }});

    // Product → manifest ownership edges, but ONLY for root manifests (those
    // with no depends_on). For chained manifests, the dep chain already
    // implies reach from the product, and adding 6 product→manifest edges
    // forces dagre to route 6 long lines that cross every task in between.
    // This is the "tangled spaghetti" failure mode from the 2026-04-23
    // screenshot — it wasn't dagre, it was that we fed it redundant edges.
    for (var mi = 0; mi < manifests.length; mi++) {
      if (!manifests[mi].depends_on) {
        elements.push({ data: { source: data.id, target: manifests[mi].id, edgeType: 'product_link' } });
      }
    }

    for (var col = 0; col < manifests.length; col++) {
      var m = manifests[col];
      var tasks = m.children || [];
      var taskCount = tasks.length;
      var completedCount = tasks.filter(function(t) { return t.status === 'completed'; }).length;

      elements.push({ data: {
        id: m.id,
        label: shortLabel(m.title),
        title: m.title, type: 'manifest', status: m.status,
        marker: m.marker, depends_on: m.depends_on || '',
        meta: JSON.stringify(m.meta || {}),
        taskInfo: completedCount + '/' + taskCount
      }});

      // Manifest → manifest edges (explicit depends_on)
      if (m.depends_on) {
        var depIds = m.depends_on.split(',').map(function(s) { return s.trim(); }).filter(Boolean);
        for (var di = 0; di < depIds.length; di++) {
          elements.push({ data: { source: depIds[di], target: m.id, edgeType: 'manifest_dep' } });
        }
      }

      // Task nodes + edges. Each task gets an edge from its actual
      // depends_on parent when that parent is inside the same manifest;
      // otherwise an ownership edge from the manifest itself. Dagre
      // handles the layout from the edge set.
      var taskIds = {};
      tasks.forEach(function(t) { taskIds[t.id] = true; });

      for (var ti = 0; ti < tasks.length; ti++) {
        var t = tasks[ti];
        // Labels render inside the 120×44 node and wrap to ~2 lines at
        // font-size 9px → ~36 chars before ellipsis kicks in.
        var shortTitle = t.title.length > 36 ? t.title.substring(0, 35) + '…' : t.title;
        elements.push({ data: {
          id: t.id, label: shortTitle,
          title: t.title, type: 'task', status: t.status,
          marker: t.marker, depends_on: t.depends_on || '',
          meta: JSON.stringify(t.meta || {})
        }});

        if (t.depends_on && taskIds[t.depends_on]) {
          elements.push({ data: { source: t.depends_on, target: t.id, edgeType: 'task_dep' } });
        } else {
          elements.push({ data: { source: m.id, target: t.id, edgeType: 'ownership' } });
        }
      }
    }
    return elements;
  };

  async function renderProductDiagram(productId) {
    var container = document.getElementById('product-cytoscape');
    if (!container || typeof cytoscape === 'undefined') return;
    container.innerHTML = '<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--text-muted)">Loading...</div>';

    try {
      var data = await fetchJSON('/api/products/' + productId + '/hierarchy');
      if (!data) return;

      var elements = OL.buildDagElements(data);

      container.innerHTML = '';
      var cy = cytoscape({
        container: container,
        elements: elements,
        // rankDir=TB: product at top, flows downward through manifests + tasks.
        // nodeSep / rankSep tuned for current label sizes — if labels grow,
        // bump these rather than reintroducing manual positions.
        // Layout invariant: ALL labels render INSIDE their node (text-valign:
        // center, text-halign: center). That means label width is bounded by
        // node width, and dagre's nodeSep directly controls label clearance.
        // Sibling labels on the same rank can never overlap regardless of
        // title length — overflow is hidden by 'text-overflow-wrap: ellipsis'.
        // Do NOT switch any node type back to text-valign:bottom — that's the
        // mode that produced the 2026-04-23 "tangled mess" regression.
        layout: {
          name: 'dagre',
          rankDir: 'TB',
          nodeSep: 30,
          rankSep: 60,
          edgeSep: 20,
          padding: 24,
          fit: true,
        },
        style: [
          { selector: 'node', style: {
              'label': 'data(label)',
              'text-wrap': 'wrap',
              'text-overflow-wrap': 'anywhere',
              'text-max-width': '110px',
              'font-size': '9px',
              'text-valign': 'center',
              'text-halign': 'center',
              'color': '#e4e4e7',
              'background-color': '#1a1a2e',
              'border-width': 2,
              'border-color': '#71717a',
              'width': 120, 'height': 44,
              'shape': 'round-rectangle',
              'padding': '6px',
          }},
          { selector: 'node[type="product"]', style: {
              'shape': 'round-rectangle',
              'width': 180, 'height': 60,
              'font-size': '12px', 'font-weight': 'bold',
              'text-max-width': '170px',
              'background-color': '#8b5cf6', 'border-color': '#8b5cf6', 'color': '#fff',
          }},
          { selector: 'node[type="manifest"]', style: {
              'shape': 'round-rectangle',
              'width': 140, 'height': 50,
              'font-size': '10px', 'font-weight': 'bold',
              'text-max-width': '130px',
              'color': '#e4e4e7',
              'background-color': '#1e3a5f',
              'border-width': 3,
              'border-color': '#3b82f6',
          }},
          { selector: 'node[status="completed"]', style: { 'border-color': '#00d97e', 'background-color': '#0a2e1a' }},
          { selector: 'node[status="running"]',   style: { 'border-color': '#f5c542', 'background-color': '#2e2a0a' }},
          { selector: 'node[status="failed"]',    style: { 'border-color': '#e63757', 'background-color': '#2e0a0a' }},
          { selector: 'edge', style: {
              'width': 1.5,
              'line-color': 'rgba(255,255,255,0.1)',
              'target-arrow-color': 'rgba(255,255,255,0.1)',
              'target-arrow-shape': 'triangle',
              'curve-style': 'straight',
              'arrow-scale': 0.7,
          }},
          { selector: 'edge[edgeType="product_link"]', style: {
              'width': 2, 'line-color': '#8b5cf6', 'target-arrow-color': '#8b5cf6', 'arrow-scale': 0.8,
          }},
          { selector: 'edge[edgeType="manifest_dep"]', style: {
              'width': 3, 'line-color': '#3b82f6', 'target-arrow-color': '#3b82f6', 'arrow-scale': 1.0,
          }},
          { selector: 'edge[edgeType="ownership"]', style: {
              'width': 1.5,
              'line-color': '#3b82f6',
              'line-style': 'dashed',
              'line-dash-pattern': [4, 4],
              'target-arrow-color': '#3b82f6',
              'arrow-scale': 0.7,
          }},
          { selector: 'edge[edgeType="task_dep"]', style: {
              'width': 1.5, 'line-color': '#f59e0b', 'target-arrow-color': '#f59e0b', 'arrow-scale': 0.7,
          }},
          { selector: 'node:active, node:selected', style: { 'border-width': 3, 'border-color': '#3b82f6', 'overlay-opacity': 0 }},
        ],
        minZoom: 0.3,
        maxZoom: 3,
        wheelSensitivity: 0.3,
      });

      // Tooltip
      var tooltip = document.createElement('div');
      tooltip.style.cssText = 'position:absolute;display:none;background:rgba(10,10,15,0.95);border:1px solid rgba(255,255,255,0.15);border-radius:6px;padding:8px 12px;font-size:11px;color:#e4e4e7;pointer-events:none;z-index:100;max-width:250px;font-family:var(--font-mono)';
      container.style.position = 'relative';
      container.appendChild(tooltip);

      cy.on('mouseover', 'node', function(e) {
        var node = e.target;
        var d = node.data();
        var meta = JSON.parse(d.meta || '{}');
        var sColor = d.status === 'completed' ? 'var(--green)' : d.status === 'running' ? '#f5c542' : d.status === 'failed' ? 'var(--red)' : 'var(--text-muted)';
        var html = '<div style="font-weight:600;margin-bottom:4px">' + esc(d.title) + '</div>';
        html += '<div><span style="color:var(--text-muted)">Status:</span> <span style="color:' + sColor + '">' + d.status + '</span></div>';
        html += '<div style="color:var(--text-muted);font-size:10px;margin-bottom:4px">' + d.marker + '</div>';
        if (d.type === 'product') {
          html += '<div>' + (meta.total_manifests || 0) + ' manifests</div>';
          html += '<div>' + (meta.total_tasks || 0) + ' tasks</div>';
        } else if (d.type === 'manifest') {
          html += '<div>' + (meta.total_tasks || 0) + ' tasks</div>';
          if (d.depends_on) {
            var depTitles = d.depends_on_titles || [];
            if (depTitles.length) {
              html += '<div style="color:#3b82f6">depends on: ' + depTitles.map(function(t) { return esc(t); }).join(', ') + '</div>';
            } else {
              html += '<div style="color:#3b82f6">depends on: ' + d.depends_on.split(',').length + ' manifest(s)</div>';
            }
          }
        } else {
          html += '<div>' + (meta.turns || 0) + ' turns</div>';
          if (meta.run_count > 0) html += '<div>' + meta.run_count + ' runs</div>';
          if (d.depends_on) {
            var depNode = cy.getElementById(d.depends_on);
            var depTitle = depNode.length ? depNode.data('title') : d.depends_on.slice(0, 8);
            html += '<div style="color:#f59e0b">depends on: ' + esc(depTitle) + '</div>';
          }
        }
        if (meta.total_cost > 0 || meta.cost_usd > 0) html += '<div><span style="color:var(--green)">$' + ((meta.total_cost || meta.cost_usd || 0).toFixed(2)) + '</span></div>';
        tooltip.innerHTML = html;
        tooltip.style.display = '';
        var pos = node.renderedPosition();
        tooltip.style.left = (pos.x + 20) + 'px';
        tooltip.style.top = (pos.y - 20) + 'px';
      });
      cy.on('mouseout', 'node', function() { tooltip.style.display = 'none'; });

      cy.on('tap', 'node', function(e) {
        var d = e.target.data();
        var diagramOverlay = document.getElementById('product-diagram-overlay');
        if (diagramOverlay) diagramOverlay.remove();
        if (d.type === 'product') {
          OL.switchView('products');
          setTimeout(function() { OL.loadProductDetail(d.id); }, 300);
        } else if (d.type === 'manifest') {
          OL.switchView('manifests');
          setTimeout(function() { OL.loadManifest(d.id); }, 300);
        } else if (d.type === 'task') {
          OL.switchView('tasks');
          setTimeout(function() { OL.loadTaskDetail(d.id); }, 300);
        }
      });

    } catch (e) {
      container.innerHTML = '<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--red)">Failed to load diagram</div>';
      console.error('Diagram error:', e);
    }
  }
})(window.OL);
