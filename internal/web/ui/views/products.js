(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  function renderProductSearchList(el, products) {
    if (!products || !products.length) {
      el.innerHTML = '<div class="empty-state" style="padding:16px">No products found</div>';
      return;
    }
    el.innerHTML = products.map(function(p) {
      var statusColors = {open:'var(--green)',closed:'var(--text-muted)',draft:'var(--yellow)',archive:'var(--red)'};
      var color = statusColors[p.status] || 'var(--text-muted)';
      var meta = [];
      if (p.total_manifests > 0) meta.push(p.total_manifests + ' manifests');
      if (p.total_tasks > 0) meta.push(p.total_tasks + ' tasks');
      if (p.total_cost > 0) meta.push('$' + p.total_cost.toFixed(2));
      return '<div class="manifest-item clickable" data-product-id="' + esc(p.id) + '" role="button" tabindex="0" ' +
        'onclick="OL.loadProductDetail(\'' + esc(p.id) + '\')" ' +
        'onkeydown="if(event.key===\'Enter\'||event.key===\' \'){event.preventDefault();this.click()}">' +
        '<div style="display:flex;align-items:center;gap:8px;margin-bottom:4px">' +
          '<span class="status-dot" style="background:' + color + '"></span>' +
          '<span class="session-uuid">' + esc(p.marker) + '</span>' +
          '<span class="badge" style="color:' + color + '">' + esc(p.status) + '</span>' +
        '</div>' +
        '<div class="manifest-item-title">' + esc(p.title) + '</div>' +
        (p.description ? '<div style="font-size:12px;color:var(--text-secondary)">' + esc(p.description) + '</div>' : '') +
        (meta.length ? '<div style="font-size:11px;color:var(--text-muted);margin-top:4px">' + meta.join(' &middot; ') + '</div>' : '') +
      '</div>';
    }).join('');
  }

  OL.loadProducts = async function() {
    var el = document.getElementById('products-list');

    var mount = document.getElementById('products-search-mount');
    if (mount && OL.mountSearchInput) {
      OL.mountSearchInput(mount, {
        placeholder: 'Search products by id, marker, tag, or keyword...',
        onSearch: async function(q) {
          var results = await fetchJSON('/api/products/search?q=' + encodeURIComponent(q));
          renderProductSearchList(el, results || []);
          return (results || []).length;
        },
        onClear: function() { OL.loadProducts(); }
      });
    }

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
          container.insertAdjacentHTML('afterbegin', '<div style="padding:8px 0;margin-bottom:8px"><button class="btn-search btn-action" id="btn-new-product">+ New Product</button></div>');
          OL.onView(container.querySelector('#btn-new-product'), 'click', function() { OL.createProduct(); });
        },
      });

      // Handle empty case — still need new product button
      if (!peerGroups || !peerGroups.length) {
        el.insertAdjacentHTML('afterbegin', '<div style="padding:8px 0;margin-bottom:8px"><button class="btn-search btn-action" id="btn-new-product">+ New Product</button></div>');
        OL.onView(el.querySelector('#btn-new-product'), 'click', function() { OL.createProduct(); });
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
        html += `<div class="amnesia-item ${statusClass}" role="button" tabindex="0" style="cursor:pointer" onclick="OL.switchView('manifests');setTimeout(()=>OL.loadManifest&&OL.loadManifest('${esc(m.id)}'),300)" onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();this.click()}">
          <div class="amnesia-header">
            <span class="amnesia-status-label">${esc(m.status)}</span>
            <span class="session-uuid">${esc(m.marker)}</span>
            <span class="meta-time">${formatTime(m.updated_at)}</span>
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
  OL.createProduct = function() {
    const titleEl = document.getElementById('product-detail-title');
    const bodyEl = document.getElementById('product-detail');
    titleEl.textContent = 'New Product';
    bodyEl.innerHTML = `
      <div style="max-width:500px">
        <div style="margin-bottom:12px">
          <label class="form-label-compact">Title *</label>
          <input id="new-product-title" class="conv-search" style="width:100%;padding:8px 12px;font-size:14px" placeholder="Product name" autofocus>
        </div>
        <div style="margin-bottom:12px">
          <label class="form-label-compact">Description</label>
          <textarea id="new-product-desc" class="conv-search" style="width:100%;min-height:80px;padding:8px 12px;font-size:13px;resize:vertical" placeholder="What is this product/project about?"></textarea>
        </div>
        <div style="margin-bottom:16px">
          <label class="form-label-compact">Tags (comma-separated)</label>
          <input id="new-product-tags" class="conv-search" style="width:100%;padding:8px 12px;font-size:13px" placeholder="e.g. openpraxis, backend">
        </div>
        <div class="flex-gap">
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

      // Fetch deps + dependents + all products for the picker.
      // Single direction=both round-trip returns everything the
      // deps section renders; all products populates the typeahead.
      let depsFromApi = [];
      let dependentsFromApi = [];
      let allProducts = [];
      try {
        const payload = await fetchJSON('/api/products/' + p.id + '/dependencies?direction=both');
        if (payload) {
          depsFromApi = payload.deps || [];
          dependentsFromApi = payload.dependents || [];
        }
      } catch(e) {}
      try {
        const groups = await fetchJSON('/api/products/by-peer');
        if (groups) {
          for (const g of groups) {
            for (const pr of (g.products || [])) allProducts.push(pr);
          }
        }
      } catch(e) {}

      // Build deps section HTML. Mirrors the manifest-deps pattern
      // from #83 so the visual grammar matches across entity tiers.
      let productDepsHtml = '';
      {
        const statusColors = {draft:'var(--yellow)',open:'var(--green)',closed:'var(--text-muted)',archive:'var(--red)'};
        const pillFor = (d, withRemove) => {
          const borderColor = statusColors[d.status] || 'var(--text-muted)';
          const removeBtn = withRemove
            ? `<span class="prod-dep-remove" data-dep-rm-id="${esc(d.id)}" style="cursor:pointer;color:var(--red);font-size:13px;line-height:1;margin-left:2px" title="Remove dependency">&times;</span>`
            : '';
          return `<span class="prod-dep-pill" data-dep-id="${esc(d.id)}" style="display:inline-flex;align-items:center;gap:4px;padding:3px 8px;border:1px solid ${borderColor};border-radius:12px;font-size:11px;font-family:var(--font-mono);margin:2px 4px 2px 0;background:var(--bg-secondary)">` +
            `<span class="prod-dep-nav" style="cursor:pointer;color:var(--accent)" data-dep-nav-id="${esc(d.id)}">${esc(d.marker)}</span>` +
            `<span style="color:var(--text-primary)">${esc(d.title)}</span>` +
            `<span style="color:${borderColor};font-size:9px;text-transform:uppercase;font-weight:600">${esc(d.status)}</span>` +
            removeBtn +
          `</span>`;
        };

        const outPills = depsFromApi.map(d => pillFor(d, true)).join('');
        const inPills = dependentsFromApi.map(d => pillFor(d, false)).join('');

        // Satisfied pill — terminal statuses are closed + archive,
        // same as the server's IsTerminalStatus.
        const terminalStatuses = ['closed', 'archive'];
        const unsatisfied = depsFromApi.filter(d => !terminalStatuses.includes(d.status));
        const satisfiedPill = depsFromApi.length === 0
          ? ''
          : (unsatisfied.length === 0
              ? `<span style="padding:1px 8px;border-radius:10px;background:rgba(0,217,126,0.15);color:var(--green);font-size:10px;font-weight:600">&#x2713; SATISFIED</span>`
              : `<span style="padding:1px 8px;border-radius:10px;background:rgba(245,158,11,0.15);color:var(--yellow);font-size:10px;font-weight:600" title="Waiting on: ${unsatisfied.map(d => d.marker).join(', ')}">&#x23F3; WAITING ON ${unsatisfied.length}</span>`);

        const depIds = depsFromApi.map(d => d.id);
        const pickerCandidates = allProducts.filter(cp =>
          cp.id !== p.id && !depIds.includes(cp.id) && cp.status !== 'archive'
        );
        const pickerOptions = pickerCandidates.map(cp =>
          `<option value="${esc(cp.id)}">${esc(cp.marker)} ${esc(cp.title)} (${esc(cp.status)})</option>`
        ).join('');

        const dependentsSection = dependentsFromApi.length > 0
          ? `<div style="margin-top:10px">
              <div style="color:var(--text-muted);font-size:12px;font-weight:500;margin-bottom:4px">Depended on by</div>
              <div style="display:flex;flex-wrap:wrap;align-items:center;min-height:24px">${inPills}</div>
            </div>`
          : '';

        productDepsHtml = `<div id="product-deps-section" style="margin-bottom:12px">
          <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
            <span style="color:var(--text-muted);font-size:12px;font-weight:500">Depends on</span>
            ${satisfiedPill}
            <button id="product-add-dep-btn" class="btn-search" style="padding:2px 10px;font-size:11px">+ Add</button>
          </div>
          <div id="product-dep-pills" style="display:flex;flex-wrap:wrap;align-items:center;min-height:24px">
            ${outPills || '<span style="font-size:11px;color:var(--text-muted);font-style:italic">No dependencies</span>'}
          </div>
          <div id="product-dep-picker" style="display:none;margin-top:6px">
            <select id="product-dep-select" class="conv-filter" style="font-size:12px;padding:4px 8px;font-family:var(--font-mono);min-width:300px">
              <option value="">Select a product...</option>
              ${pickerOptions}
            </select>
            <div id="product-dep-error" style="display:none;margin-top:4px;color:var(--red);font-size:11px"></div>
          </div>
          ${dependentsSection}
        </div>`;
      }

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
              <div role="button" tabindex="0" style="flex:1;cursor:pointer" onclick="OL.switchView('manifests');setTimeout(()=>OL.loadManifest('${esc(m.id)}'),300)" onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();this.click()}">
                <div class="flex-row">
                  <span class="session-uuid" style="font-size:11px">${esc(m.marker)}</span>
                  <span class="badge ${mStatusClass} badge-sm">${esc(m.status)}</span>
                  ${mCostParts.length ? mCostParts.join('<span style="opacity:0.3;font-size:10px"> | </span>') : ''}
                </div>
                <div style="font-size:13px;color:var(--text-primary);margin-top:4px">${esc(m.title)}</div>
              </div>
              <button class="btn-dismiss" style="font-size:10px;padding:2px 8px;flex-shrink:0" onclick="event.stopPropagation();OL.unlinkManifestFromProduct('${esc(m.id)}','${esc(p.id)}')" title="Remove from product" aria-label="Remove from product">&#x2715;</button>
            </div>`;
          }).join('');
          manifestsHtml = `<div class="section-divider">
            <div class="section-title">Linked Manifests <span class="sub-count">(${manifests.length})</span></div>
            <div class="bordered-container">${manifestRows}</div>
          </div>`;
        } else {
          manifestsHtml = `<div class="section-divider">
            <div class="section-title">Linked Manifests</div>
            <div class="empty-placeholder">No manifests linked yet</div>
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
                <div class="flex-row">
                  <span class="session-uuid" style="font-size:11px">${esc(i.marker)}</span>
                  <span class="badge badge-sm">${esc(i.status)}</span>
                  <span style="font-size:10px;color:${prioColor}">${esc(i.priority)}</span>
                </div>
                <div style="font-size:13px;color:var(--text-primary);margin-top:2px">${esc(i.title)}</div>
              </div>
              <button class="btn-dismiss" style="font-size:10px;padding:2px 8px;flex-shrink:0" onclick="event.stopPropagation();OL.unlinkIdeaFromProduct('${esc(i.id)}','${esc(p.id)}')" title="Remove from product" aria-label="Remove from product">&#x2715;</button>
            </div>`;
          }).join('');
          ideasHtml = `<div class="section-divider">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span style="font-size:13px;color:var(--text-primary);font-weight:600">Linked Ideas <span class="sub-count">(${ideas.length})</span></span>
              <button class="btn-search btn-xs" onclick="OL.linkIdeaToProduct('${esc(p.id)}')">+ Link Idea</button>
            </div>
            <div class="bordered-container">${ideaRows}</div>
          </div>`;
        } else {
          ideasHtml = `<div class="section-divider">
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span style="font-size:13px;color:var(--text-primary);font-weight:600">Linked Ideas</span>
              <button class="btn-search btn-xs" onclick="OL.linkIdeaToProduct('${esc(p.id)}')">+ Link Idea</button>
            </div>
            <div class="empty-placeholder">No ideas linked yet</div>
          </div>`;
        }
      } catch(e) {}

      bodyEl.innerHTML = `
        <div>
          <!-- BREADCRUMB -->
          <div class="breadcrumb">
            <span class="breadcrumb-link" onclick="OL.switchView('products')">${esc(p.source_node ? p.source_node.substring(0,12) : 'node')}</span>
            <span class="breadcrumb-sep"> &rarr; </span>
            <span style="color:var(--text-primary)">${esc(p.marker)} ${esc(p.title)}</span>
          </div>
          <!-- METADATA BAR -->
          <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:16px">
            <span class="session-uuid" style="font-size:14px">${esc(p.marker)}</span>
            <span class="badge ${statusClass}">${esc(p.status)}</span>
            ${tags}
            <button class="btn-copy" onclick="OL.copy('get product ${esc(p.marker)}')" title="Copy ref" aria-label="Copy reference">&#x2398;</button>
          </div>
          <!-- METRICS BAR -->
          <div class="stats-bar">
            <span>Manifests: <strong style="color:var(--text-primary)">${p.total_manifests || 0}</strong></span>
            <span>Tasks: <strong style="color:var(--text-primary)">${p.total_tasks || 0}</strong></span>
            <span>Turns: <strong style="color:var(--text-primary)">${p.total_turns || 0}</strong></span>
            <span>Cost: <strong style="color:var(--green)">$${(p.total_cost || 0).toFixed(2)}</strong></span>
            <span class="separator">|</span>
            <span>Created: ${new Date(p.created_at).toLocaleString()}</span>
            <span>Updated: ${new Date(p.updated_at).toLocaleString()}</span>
          </div>
          <!-- CONTROLS -->
          <div style="display:flex;gap:8px;margin-bottom:12px;flex-wrap:wrap">
            <button class="btn-search" style="font-size:11px;padding:4px 12px" onclick="OL.editProduct('${esc(p.id)}')">&#9998; Edit</button>
            <button class="btn-search" style="font-size:11px;padding:4px 12px" onclick="OL.createManifest({productId:'${esc(p.id)}'})">+ New Manifest</button>
            <button class="btn-search" style="font-size:11px;padding:4px 12px;background:var(--bg-input)" onclick="OL.linkManifestToProduct('${esc(p.id)}')">+ Link Manifest</button>
            ${['draft','open','closed','archive'].map(s => {
              const active = p.status === s;
              const color = s === 'open' ? 'var(--green)' : s === 'closed' ? 'var(--text-muted)' : s === 'archive' ? 'var(--red)' : 'var(--yellow)';
              return '<button class="product-status-btn" data-status="' + s + '" style="padding:4px 12px;font-size:11px;font-weight:600;text-transform:uppercase;border-radius:4px;cursor:pointer;border:1px solid ' + color + ';background:' + (active ? color : 'transparent') + ';color:' + (active ? 'var(--bg-primary)' : color) + ';opacity:' + (active ? '1' : '0.7') + '" onclick="OL.updateProductStatus(\'' + esc(p.id) + '\',\'' + s + '\')">' + s + '</button>';
            }).join('')}
          </div>
          ${p.description ? `<div style="font-size:13px;color:var(--text-secondary);line-height:1.6;margin-bottom:12px;white-space:pre-wrap">${esc(p.description)}</div>` : ''}
          ${productDepsHtml}
          <!-- DIAGRAM BUTTON -->
          <div style="margin-bottom:12px">
            <button class="btn-search btn-action" onclick="OL.showProductDiagram('${esc(p.id)}','${esc(p.title)}')">&#x25C8; Product DAG</button>
          </div>
          ${manifestsHtml}
          <div id="product-knobs-mount" style="margin-top:16px"></div>
          <div id="product-comments-mount" style="margin-top:16px"></div>
          ${ideasHtml}
        </div>`;

      const knobMount = document.getElementById('product-knobs-mount');
      if (knobMount && OL.renderKnobSection) {
        OL.renderKnobSection(knobMount, { type: 'product', id: p.id });
      }

      const commentsMount = document.getElementById('product-comments-mount');
      if (commentsMount && OL.renderCommentsSection) {
        OL.renderCommentsSection(commentsMount, { type: 'product', id: p.id });
      }

      // Wire dep pill nav: click a marker → open that product.
      bodyEl.querySelectorAll('.prod-dep-nav').forEach(el => {
        el.addEventListener('click', (e) => {
          e.stopPropagation();
          OL.loadProduct(el.dataset.depNavId);
        });
      });

      // Wire dep remove buttons — DELETE via the single-edge endpoint.
      // Server hits #88's RemoveDep which (in a follow-up PR) will
      // trigger the same rehab pattern as manifest-dep removal (#79).
      bodyEl.querySelectorAll('.prod-dep-remove').forEach(el => {
        el.addEventListener('click', async (e) => {
          e.stopPropagation();
          const removeId = el.dataset.depRmId;
          try {
            const res = await fetch('/api/products/' + p.id + '/dependencies/' + removeId, {
              method: 'DELETE'
            });
            if (!res.ok && res.status !== 204) {
              console.error('Remove product dep failed:', res.status);
              return;
            }
            OL.loadProduct(p.id);
          } catch(err) {
            console.error('Remove product dep failed:', err);
          }
        });
      });

      // Wire add-dep button + picker. 409 = cycle, 400 = self-loop/bad body,
      // 404 = missing product. The server error surfaces inline in red.
      const addDepBtn = document.getElementById('product-add-dep-btn');
      const depPicker = document.getElementById('product-dep-picker');
      const depSelect = document.getElementById('product-dep-select');
      const depError = document.getElementById('product-dep-error');
      const showDepError = (msg) => {
        if (!depError) return;
        depError.textContent = msg;
        depError.style.display = msg ? 'block' : 'none';
      };
      if (addDepBtn && depPicker && depSelect) {
        addDepBtn.addEventListener('click', () => {
          const visible = depPicker.style.display !== 'none';
          depPicker.style.display = visible ? 'none' : 'block';
          showDepError('');
          if (!visible) depSelect.focus();
        });
        depSelect.addEventListener('change', async () => {
          const selectedId = depSelect.value;
          if (!selectedId) return;
          showDepError('');
          try {
            const res = await fetch('/api/products/' + p.id + '/dependencies', {
              method: 'POST',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({depends_on_id: selectedId})
            });
            if (!res.ok) {
              const body = await res.json().catch(() => ({}));
              showDepError(body.error || ('Add failed: HTTP ' + res.status));
              depSelect.value = '';
              return;
            }
            OL.loadProduct(p.id);
          } catch(err) {
            showDepError('Add failed: ' + (err && err.message ? err.message : err));
          }
        });
      }

    } catch (e) {
      console.error('Load product detail failed:', e);
    }
  };

  OL.loadProduct = OL.loadProductDetail;

  // --- Cytoscape Directed Acyclic Graph Diagram (Full Page) ---
  OL.showProductDiagram = function(productId, productTitle) {
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
          <span style="display:flex;align-items:center;gap:4px"><span style="display:inline-block;width:20px;height:0;border-top:2.5px solid #3b82f6"></span>manifest dep</span>
          <span style="display:flex;align-items:center;gap:4px"><span style="display:inline-block;width:20px;height:0;border-top:2px dashed #f59e0b"></span>task dep</span>
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

      // Nodes + edges, no manual positions. Layout is delegated to the
      // dagre cytoscape plugin (loaded in index.html alongside cytoscape),
      // which ranks + places automatically for any DAG shape — linear
      // chains, independent pairs, multi-parent fan-in, empty manifests,
      // all handled without layout-specific code here.
      //
      // This replaced three rounds of hand-rolled position math
      // (preset layout + column/row arithmetic + DFS task ordering) that
      // each broke as soon as a new product DAG shape showed up.
      const elements = [];
      const manifests = data.children || [];

      function shortLabel(title) {
        return title.replace(/^QA\s+/, '').replace(/^OpenPraxis\s+/, '').replace(/\s*—\s*.+$/, '');
      }

      var statusColor = function(status) {
        if (status === 'completed') return '#00d97e';
        if (status === 'running') return '#f5c542';
        if (status === 'failed') return '#e63757';
        if (status === 'closed' || status === 'archive') return '#00d97e';
        if (status === 'open') return '#3b82f6';
        if (status === 'draft') return '#f5c542';
        return '#71717a';
      };

      // Product node
      elements.push({ data: {
        id: data.id, label: data.title, title: data.title, type: 'product',
        status: data.status, marker: data.marker,
        meta: JSON.stringify(data.meta || {})
      }});

      // Product → manifest edges (ownership)
      for (var mi = 0; mi < manifests.length; mi++) {
        elements.push({ data: { source: data.id, target: manifests[mi].id, edgeType: 'product_link' } });
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
        // depends_on parent (when that parent is inside the same manifest),
        // otherwise an ownership edge from the manifest itself. Dagre
        // figures out the layout from the edge set.
        var taskIds = {};
        tasks.forEach(function(t) { taskIds[t.id] = true; });

        for (var ti = 0; ti < tasks.length; ti++) {
          var t = tasks[ti];
          var shortTitle = t.title.length > 25 ? t.title.substring(0, 23) + '…' : t.title;
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

      container.innerHTML = '';
      const cy = cytoscape({
        container: container,
        elements: elements,
        // dagre: hierarchical topological layout. rankDir=TB puts product
        // at the top and flows downward through manifests and tasks.
        // nodeSep / rankSep tuned for the current label sizes; if labels
        // grow, bump these rather than going back to manual positions.
        layout: {
          name: 'dagre',
          rankDir: 'TB',
          nodeSep: 40,
          rankSep: 70,
          edgeSep: 20,
          padding: 24,
          fit: true,
        },
        style: [
          {
            selector: 'node',
            style: {
              'label': 'data(label)',
              'text-wrap': 'wrap',
              'text-max-width': '140px',
              'font-size': '9px',
              'text-valign': 'bottom',
              'text-halign': 'center',
              'text-margin-y': 8,
              'color': '#a1a1aa',
              'background-color': '#1a1a2e',
              'border-width': 2,
              'border-color': '#71717a',
              'width': 30,
              'height': 30,
              'shape': 'ellipse',
              'text-outline-color': '#0a0a0f',
              'text-outline-width': 1,
            }
          },
          {
            selector: 'node[type="product"]',
            style: {
              'shape': 'round-rectangle',
              'width': 70, 'height': 50,
              'font-size': '12px', 'font-weight': 'bold',
              'text-max-width': '160px',
              'text-valign': 'center', 'text-halign': 'center', 'text-margin-y': 0,
              'background-color': '#8b5cf6', 'border-color': '#8b5cf6', 'color': '#fff',
            }
          },
          {
            selector: 'node[type="manifest"]',
            style: {
              'shape': 'round-rectangle',
              'width': 55,
              'height': 40,
              'font-size': '10px',
              'font-weight': 'bold',
              'text-max-width': '150px',
              'text-valign': 'center',
              'text-halign': 'center',
              'text-margin-y': 0,
              'color': '#e4e4e7',
              'background-color': '#1e3a5f',
              'border-width': 3,
              'border-color': '#3b82f6',
            }
          },
          {
            selector: 'node[status="completed"]',
            style: { 'border-color': '#00d97e', 'background-color': '#0a2e1a' }
          },
          {
            selector: 'node[status="running"]',
            style: { 'border-color': '#f5c542', 'background-color': '#2e2a0a' }
          },
          {
            selector: 'node[status="failed"]',
            style: { 'border-color': '#e63757', 'background-color': '#2e0a0a' }
          },
          {
            selector: 'edge',
            style: {
              'width': 1.5,
              'line-color': 'rgba(255,255,255,0.1)',
              'target-arrow-color': 'rgba(255,255,255,0.1)',
              'target-arrow-shape': 'triangle',
              'curve-style': 'straight',
              'arrow-scale': 0.7,
            }
          },
          {
            selector: 'edge[edgeType="product_link"]',
            style: {
              'width': 2,
              'line-color': '#8b5cf6',
              'target-arrow-color': '#8b5cf6',
              'target-arrow-shape': 'triangle',
              'curve-style': 'straight',
              'arrow-scale': 0.8,
            }
          },
          {
            selector: 'edge[edgeType="manifest_dep"]',
            style: {
              'width': 3,
              'line-color': '#3b82f6',
              'target-arrow-color': '#3b82f6',
              'target-arrow-shape': 'triangle',
              'curve-style': 'straight',
              'arrow-scale': 1.0,
            }
          },
          {
            selector: 'edge[edgeType="ownership"]',
            style: {
              'width': 1.5,
              'line-color': '#3b82f6',
              'line-style': 'dashed',
              'line-dash-pattern': [4, 4],
              'target-arrow-color': '#3b82f6',
              'target-arrow-shape': 'triangle',
              'curve-style': 'straight',
              'arrow-scale': 0.7,
            }
          },
          {
            selector: 'edge[edgeType="task_dep"]',
            style: {
              'width': 1.5,
              'line-color': '#f59e0b',
              'target-arrow-color': '#f59e0b',
              'target-arrow-shape': 'triangle',
              'curve-style': 'straight',
              'arrow-scale': 0.7,
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
        var sColor = d.status === 'completed' ? 'var(--green)' : d.status === 'running' ? '#f5c542' : d.status === 'failed' ? 'var(--red)' : 'var(--text-muted)';
        let html = `<div style="font-weight:600;margin-bottom:4px">${esc(d.title)}</div>`;
        html += `<div><span style="color:var(--text-muted)">Status:</span> <span style="color:${sColor}">${d.status}</span></div>`;
        html += `<div style="color:var(--text-muted);font-size:10px;margin-bottom:4px">${d.marker}</div>`;
        if (d.type === 'product') {
          html += `<div>${meta.total_manifests || 0} manifests</div>`;
          html += `<div>${meta.total_tasks || 0} tasks</div>`;
        } else if (d.type === 'manifest') {
          html += `<div>${meta.total_tasks || 0} tasks</div>`;
          if (d.depends_on) {
            const depTitles = d.depends_on_titles || [];
            if (depTitles.length) {
              html += `<div style="color:#3b82f6">depends on: ${depTitles.map(t => esc(t)).join(', ')}</div>`;
            } else {
              html += `<div style="color:#3b82f6">depends on: ${d.depends_on.split(',').length} manifest(s)</div>`;
            }
          }
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
          setTimeout(() => OL.loadManifest(d.id), 300);
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
  OL.editProduct = function(id) {
    const titleEl = document.getElementById('product-detail-title');
    const bodyEl = document.getElementById('product-detail');
    fetchJSON('/api/products/' + id).then(p => {
      if (!p) return;
      titleEl.textContent = 'Edit Product';
      bodyEl.innerHTML = `
        <div style="max-width:500px">
          <div style="margin-bottom:12px">
            <label class="form-label-compact">Title</label>
            <input id="edit-product-title" class="conv-search" style="width:100%;padding:8px 12px;font-size:14px" value="${esc(p.title)}">
          </div>
          <div style="margin-bottom:12px">
            <label class="form-label-compact">Description</label>
            <textarea id="edit-product-desc" class="conv-search" style="width:100%;min-height:80px;padding:8px 12px;font-size:13px;resize:vertical">${esc(p.description)}</textarea>
          </div>
          <div style="margin-bottom:16px">
            <label class="form-label-compact">Tags (comma-separated)</label>
            <input id="edit-product-tags" class="conv-search" style="width:100%;padding:8px 12px;font-size:13px" value="${esc((p.tags||[]).join(', '))}">
          </div>
          <div class="flex-gap">
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

  OL.updateProductStatus = async function(id, status) {
    await fetchJSON('/api/products/' + id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({status}) });
    OL.loadProducts();
    OL.loadProductDetail(id);
  };

  // _deleteProduct removed -- use status toggle to archive instead

  OL.linkManifestToProduct = async function(productId) {
    // Fetch all manifests not yet linked to this product
    const allManifests = await fetchJSON('/api/manifests');
    const unlinked = (allManifests || []).filter(m => m.project_id !== productId);
    if (!unlinked.length) { alert('No manifests available to link'); return; }

    const bodyEl = document.getElementById('product-detail');
    const origHtml = bodyEl.innerHTML;
    let listHtml = unlinked.map(m =>
      `<div class="manifest-item clickable" data-link-mid="${esc(m.id)}" role="button" tabindex="0" style="padding:8px 12px;border-bottom:1px solid var(--border)">
        <span class="session-uuid" style="font-size:11px">${esc(m.marker)}</span>
        <span class="badge badge-sm">${esc(m.status)}</span>
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

  OL.unlinkManifestFromProduct = async function(manifestId, productId) {
    await fetchJSON('/api/manifests/' + manifestId, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: ''}) });
    OL.loadProducts();
    OL.loadProductDetail(productId);
  };

  OL.linkIdeaToProduct = async function(productId) {
    const allIdeas = await fetchJSON('/api/ideas');
    const unlinked = (allIdeas || []).filter(i => i.project_id !== productId);
    if (!unlinked.length) { alert('No ideas available to link'); return; }

    const bodyEl = document.getElementById('product-detail');
    let listHtml = unlinked.map(i =>
      `<div class="manifest-item clickable" data-link-iid="${esc(i.id)}" role="button" tabindex="0" style="padding:8px 12px;border-bottom:1px solid var(--border)">
        <span class="session-uuid" style="font-size:11px">${esc(i.marker)}</span>
        <span class="badge badge-sm">${esc(i.status)}</span>
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

  OL.unlinkIdeaFromProduct = async function(ideaId, productId) {
    const idea = await fetchJSON('/api/ideas/' + ideaId);
    if (idea) {
      await fetchJSON('/api/ideas/' + ideaId, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({project_id: '', title: idea.title, description: idea.description, status: idea.status, priority: idea.priority}) });
    }
    OL.loadProducts();
    OL.loadProductDetail(productId);
  };

})(window.OL);
