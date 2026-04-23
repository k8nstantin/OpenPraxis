(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  OL.loadManifests = async function() {
    const el = document.getElementById('manifest-list');
    const newBtn = document.getElementById('manifest-new-btn');

    if (newBtn) newBtn.onclick = () => OL.createManifest();

    const mount = document.getElementById('manifest-search-mount');
    if (mount && OL.mountSearchInput) {
      OL.mountSearchInput(mount, {
        placeholder: 'Search manifests by id, marker, Jira ref, tag, or keyword...',
        onSearch: async function(q) {
          const results = await fetchJSON('/api/manifests/search?q=' + encodeURIComponent(q));
          renderManifestList(el, results || []);
          return (results || []).length;
        },
        onClear: function() { OL.loadManifests(); }
      });
    }

    try {
      var results = await Promise.all([
        fetchJSON('/api/manifests/by-peer'),
        fetchJSON('/api/products'),
      ]);
      var peerGroups = results[0];
      var products = results[1];

      // Build product lookup
      var prodMap = {};
      for (var pi = 0; pi < (products || []).length; pi++) {
        prodMap[products[pi].id] = products[pi];
      }

      // Transform: group manifests by product within each peer
      var treeData = (peerGroups || []).map(function(pg) {
        var byProduct = {};
        var productOrder = [];
        for (var i = 0; i < pg.manifests.length; i++) {
          var m = pg.manifests[i];
          var pid = (m.project_id && prodMap[m.project_id]) ? m.project_id : '__unassigned__';
          if (!byProduct[pid]) {
            byProduct[pid] = { pid: pid, prod: prodMap[pid] || null, items: [] };
            productOrder.push(pid);
          }
          byProduct[pid].items.push(m);
        }
        return {
          peer_id: pg.peer_id,
          count: pg.count,
          productGroups: productOrder.map(function(p) { return byProduct[p]; }),
        };
      });

      OL.renderTree(el, treeData, {
        prefix: 'mfst',
        emptyMessage: 'No manifests yet',
        levels: [
          {
            label: function(pg) { return esc(pg.peer_id); },
            count: function(pg) { return pg.count; },
            children: function(pg) { return pg.productGroups; },
          },
          {
            label: function(grp) { return esc(grp.prod ? grp.prod.title : 'Unassigned'); },
            extra: function(grp) {
              return grp.prod && grp.prod.marker ? '<span class="session-uuid">' + esc(grp.prod.marker) + '</span>' : '';
            },
            count: function(grp) { return grp.items.length; },
            children: function(grp) { return grp.items; },
            dotColor: function(grp) { return grp.prod ? 'green' : ''; },
            expanded: false,
          }
        ],
        renderLeaf: function(m) {
          var statusClass = m.status === 'open' ? 'confirmed' : m.status === 'closed' ? 'flagged' : 'dismissed';
          var metaParts = [];
          if (m.total_tasks > 0) metaParts.push(m.total_tasks + ' tasks');
          if (m.total_turns > 0) metaParts.push(m.total_turns + ' turns');
          if (m.total_cost > 0) metaParts.push('$' + m.total_cost.toFixed(2));

          return '<div class="amnesia-item ' + statusClass + ' clickable tree-leaf tree-indent" data-id="' + esc(m.id) + '">' +
            '<div class="amnesia-header">' +
              '<span class="amnesia-status-label">' + esc(m.status) + '</span>' +
              '<span class="session-uuid">' + esc(m.marker) + '</span>' +
              (metaParts.length ? '<span style="font-size:11px;color:var(--text-muted)">' + metaParts.join(' &middot; ') + '</span>' : '') +
              '<span class="meta-time">' + formatTime(m.updated_at || '') + '</span>' +
            '</div>' +
            '<div class="amnesia-rule">' + esc(m.title) + '</div>' +
          '</div>';
        },
        leafSelector: '.tree-leaf',
        onLeafClick: function(item) { OL.loadManifest(item.dataset.id); },
      });
    } catch (e) {
      console.error('Load manifests failed:', e);
    }
  };

  function renderManifestList(el, manifests) {
    if (!manifests || !manifests.length) {
      el.innerHTML = '<div class="empty-state">No manifests found</div>';
      return;
    }
    el.innerHTML = manifests.map(m => {
      const statusClass = m.status === 'open' ? 'scope' : m.status === 'closed' ? 'type' : m.status === 'archive' ? 'type' : '';
      const jira = (m.jira_refs || []).join(', ');
      return `<div class="manifest-item clickable" data-id="${esc(m.id)}" role="button" tabindex="0" onclick="OL.loadManifest('${esc(m.id)}')" onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();this.click()}">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:4px">
          <span class="session-uuid">${esc(m.marker)}</span>
          <span class="badge ${statusClass}">${esc(m.status)}</span>
          <span style="font-size:11px;color:var(--text-muted)">v${m.version}</span>
          <button class="btn-copy-sm" onclick="event.stopPropagation();OL.copy('get manifest ${esc(m.marker)}')" title="Copy ref" aria-label="Copy reference">&#x2398;</button>
        </div>
        <div class="manifest-item-title">${esc(m.title)}</div>
        <div style="font-size:12px;color:var(--text-secondary)">${esc(m.description)}</div>
        ${jira ? `<div style="font-size:11px;color:var(--accent);margin-top:4px">${esc(jira)}</div>` : ''}
      </div>`;
    }).join('');
  }

  // Open the "New Manifest" form in the manifest detail panel.
  // opts: { title, description, content, productId } — all optional.
  OL.createManifest = async function(opts) {
    opts = opts || {};
    const title = opts.title || '';
    const description = opts.description || '';
    const content = opts.content || '';
    const preselectProductId = opts.productId || '';

    OL.switchView('manifests');

    let products = [];
    try {
      const groups = await fetchJSON('/api/products/by-peer');
      if (groups) {
        for (const g of groups) {
          for (const p of (g.products || [])) {
            products.push(p);
          }
        }
      }
    } catch(e) {}

    setTimeout(() => {
      const titleEl = document.getElementById('manifest-detail-title');
      const bodyEl = document.getElementById('manifest-detail');
      titleEl.textContent = 'New Manifest';

      const productOptions = products.map(p =>
        `<option value="${esc(p.id)}"${p.id === preselectProductId ? ' selected' : ''}>${esc(p.marker)} ${esc(p.title)}</option>`
      ).join('');

      bodyEl.innerHTML = `
        <div style="max-width:600px">
          <div style="margin-bottom:16px">
            <label class="form-label">Title</label>
            <input type="text" id="pm-title" class="conv-search" style="font-size:14px" value="${esc(title)}" />
          </div>
          <div style="margin-bottom:16px">
            <label class="form-label">Description</label>
            <input type="text" id="pm-description" class="conv-search" style="font-size:13px" value="${esc(description)}" />
          </div>
          <div style="margin-bottom:16px">
            <label class="form-label">Content</label>
            <textarea id="pm-content" class="conv-search" style="font-size:13px;height:200px;resize:vertical;font-family:var(--font-mono)">${esc(content)}</textarea>
          </div>
          <div style="display:flex;gap:12px;margin-bottom:16px">
            <div style="flex:1">
              <label class="form-label">Status</label>
              <select id="pm-status" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
                <option value="draft" selected>Draft</option>
                <option value="open">Open</option>
                <option value="closed">Closed</option>
              </select>
            </div>
            <div style="flex:1">
              <label class="form-label">Product</label>
              <select id="pm-product-id" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
                <option value=""${preselectProductId ? '' : ' selected'}>No Product</option>
                ${productOptions}
              </select>
            </div>
          </div>
          <div style="margin-bottom:16px">
            <label class="form-label">Jira Refs</label>
            <input type="text" id="pm-jira" class="conv-search" placeholder="ENG-1234, ENG-5678" style="font-size:13px" />
          </div>
          <div class="flex-gap">
            <button id="pm-submit" class="btn-search" style="padding:8px 20px">Create Manifest</button>
            <span id="pm-status-msg" style="font-size:13px;color:var(--green);align-self:center"></span>
          </div>
        </div>
      `;

      bodyEl.querySelector('#pm-submit').onclick = async () => {
        const t = bodyEl.querySelector('#pm-title').value.trim();
        const d = bodyEl.querySelector('#pm-description').value.trim();
        const c = bodyEl.querySelector('#pm-content').value.trim();
        const s = bodyEl.querySelector('#pm-status').value;
        const pid = bodyEl.querySelector('#pm-product-id').value;
        const j = bodyEl.querySelector('#pm-jira').value.trim();
        if (!t) { bodyEl.querySelector('#pm-status-msg').textContent = 'Title required'; return; }

        const resp = await fetch('/api/manifests', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({title: t, description: d, content: c, status: s, project_id: pid, jira_refs: j ? j.split(',').map(s => s.trim()) : []})
        });
        if (!resp.ok) { bodyEl.querySelector('#pm-status-msg').textContent = 'Error: ' + resp.status; return; }
        const m = await resp.json();
        bodyEl.querySelector('#pm-status-msg').textContent = 'Created!';
        OL.loadManifests();
        setTimeout(() => OL.loadManifest(m.id), 500);
      };
    }, 300);
  };

  // Promote idea or memory to manifest -- opens manifest detail panel with creation form pre-filled.
  OL.promoteToManifest = function(title, description, content) {
    return OL.createManifest({ title: title, description: description, content: content });
  };

  OL.loadManifest = async function(id) {
    document.querySelectorAll('.manifest-item').forEach(i => i.classList.remove('active'));
    const active = document.querySelector(`.manifest-item[data-id="${id}"]`);
    if (active) active.classList.add('active');

    try {
      const m = await fetchJSON('/api/manifests/' + id);
      if (!m) return;

      const titleEl = document.getElementById('manifest-detail-title');
      const bodyEl = document.getElementById('manifest-detail');

      const jira = (m.jira_refs || []).map(r => `<a href="https://gryphonnetworks.atlassian.net/browse/${r}" target="_blank" style="color:var(--accent)">${esc(r)}</a>`).join(', ');
      const tags = (m.tags || []).map(t => `<span class="badge tag">${esc(t)}</span>`).join(' ');

      titleEl.innerHTML = `<span id="manifest-edit-title" class="manifest-editable" style="cursor:pointer;border-radius:4px;padding:2px 4px" title="Click to edit title">${esc(m.title)}</span> <button class="btn-copy" onclick="OL.copy('get manifest ${esc(m.marker)}')" title="Copy ref" aria-label="Copy reference">&#x2398;</button>`;

      // Fetch linked product for breadcrumb
      let product = null;
      if (m.project_id) {
        try { product = await fetchJSON('/api/products/' + m.project_id); } catch(e) {}
      }

      // Fetch all products for assignment dropdown
      let allProducts = [];
      try {
        const groups = await fetchJSON('/api/products/by-peer');
        if (groups) {
          for (const g of groups) {
            for (const p of (g.products || [])) {
              allProducts.push(p);
            }
          }
        }
      } catch(e) {}

      // Fetch all manifests for dependency picker
      let allManifests = [];
      try {
        const manifestList = await fetchJSON('/api/manifests');
        if (manifestList) allManifests = manifestList;
      } catch(e) {}

      // Fetch deps + dependents from the authoritative endpoint
      // (#81). The legacy m.depends_on comma-string is still populated
      // for backwards compat, but we read the join-table view here so
      // the UI sees cycle-validated edges plus the in-edges the legacy
      // column doesn't carry. direction=both gives us everything in
      // one round trip.
      let depsFromApi = [];
      let dependentsFromApi = [];
      try {
        const depPayload = await fetchJSON('/api/manifests/' + m.id + '/dependencies?direction=both');
        if (depPayload) {
          depsFromApi = depPayload.deps || [];
          dependentsFromApi = depPayload.dependents || [];
        }
      } catch(e) {}

      // IDs this manifest already depends on — used to filter the picker.
      const depIds = depsFromApi.map(d => d.id);

      let depsHtml = '';
      {
        const statusColors = {draft:'var(--yellow)',open:'var(--green)',closed:'var(--text-muted)',archive:'var(--red)'};

        const pillFor = (d, withRemove) => {
          const borderColor = statusColors[d.status] || 'var(--text-muted)';
          const removeBtn = withRemove
            ? `<span class="dep-remove" data-dep-rm-id="${esc(d.id)}" style="cursor:pointer;color:var(--red);font-size:13px;line-height:1;margin-left:2px" title="Remove dependency">&times;</span>`
            : '';
          return `<span class="dep-pill" data-dep-id="${esc(d.id)}" style="display:inline-flex;align-items:center;gap:4px;padding:3px 8px;border:1px solid ${borderColor};border-radius:12px;font-size:11px;font-family:var(--font-mono);margin:2px 4px 2px 0;background:var(--bg-secondary)">` +
            `<span class="dep-pill-nav" style="cursor:pointer;color:var(--accent)" data-dep-nav-id="${esc(d.id)}">${esc(d.marker)}</span>` +
            `<span style="color:var(--text-primary)">${esc(d.title)}</span>` +
            `<span style="color:${borderColor};font-size:9px;text-transform:uppercase;font-weight:600">${esc(d.status)}</span>` +
            removeBtn +
          `</span>`;
        };

        const outPills = depsFromApi.map(d => pillFor(d, true)).join('');
        const inPills = dependentsFromApi.map(d => pillFor(d, false)).join('');

        // Satisfied indicator: every out-edge must be terminal (closed/archive).
        // Mirror the server's IsTerminalStatus — keep the set in sync manually
        // until a dedicated endpoint exposes it.
        const terminalStatuses = ['closed', 'archive'];
        const unsatisfied = depsFromApi.filter(d => !terminalStatuses.includes(d.status));
        const satisfiedPill = depsFromApi.length === 0
          ? ''
          : (unsatisfied.length === 0
              ? `<span style="padding:1px 8px;border-radius:10px;background:rgba(0,217,126,0.15);color:var(--green);font-size:10px;font-weight:600">&#x2713; SATISFIED</span>`
              : `<span style="padding:1px 8px;border-radius:10px;background:rgba(245,158,11,0.15);color:var(--yellow);font-size:10px;font-weight:600" title="Waiting on: ${unsatisfied.map(d => d.marker).join(', ')}">&#x23F3; WAITING ON ${unsatisfied.length}</span>`);

        // Picker candidates: exclude self + already-linked; include ALL
        // manifests (same-product filter dropped — cross-product deps
        // are explicitly allowed per #74 design note).
        const pickerCandidates = allManifests.filter(cm =>
          cm.id !== m.id &&
          !depIds.includes(cm.id) &&
          cm.status !== 'archive'
        );
        const pickerOptions = pickerCandidates.map(cm =>
          `<option value="${esc(cm.id)}">${esc(cm.marker)} ${esc(cm.title)} (${esc(cm.status)})</option>`
        ).join('');

        const dependentsSection = dependentsFromApi.length > 0
          ? `<div style="margin-top:10px">
              <div style="color:var(--text-muted);font-size:12px;font-weight:500;margin-bottom:4px">Depended on by</div>
              <div style="display:flex;flex-wrap:wrap;align-items:center;min-height:24px">${inPills}</div>
            </div>`
          : '';

        depsHtml = `<div id="manifest-deps-section" style="margin-bottom:12px">
          <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
            <span style="color:var(--text-muted);font-size:12px;font-weight:500">Depends on</span>
            ${satisfiedPill}
          </div>
          <div id="manifest-dep-pills" style="display:flex;flex-wrap:wrap;align-items:center;min-height:24px">
            ${outPills || '<span style="font-size:11px;color:var(--text-muted);font-style:italic">No dependencies</span>'}
          </div>
          <div id="manifest-dep-picker" style="display:none;margin-top:6px">
            <select id="manifest-dep-select" class="conv-filter" style="font-size:12px;padding:4px 8px;font-family:var(--font-mono);min-width:300px">
              <option value="">Select a manifest...</option>
              ${pickerOptions}
            </select>
            <div id="manifest-dep-error" style="display:none;margin-top:4px;color:var(--red);font-size:11px"></div>
          </div>
          ${dependentsSection}
        </div>`;
      }

      // Fetch linked ideas
      let linkedIdeasHtml = '';
      try {
        const linkedIdeas = await fetchJSON('/api/manifests/' + m.id + '/ideas');
        if (linkedIdeas && linkedIdeas.length) {
          linkedIdeasHtml = '<div style="margin-bottom:12px"><span style="color:var(--text-muted);font-size:12px">Ideas:</span> ' +
            linkedIdeas.map(i => `<span class="badge idea-nav" style="cursor:pointer;color:var(--green)" data-iid="${i.marker}">${esc(i.marker)} ${esc(i.title)}</span>`).join(' ') +
            '</div>';
        }
      } catch(e) {}

      // Fetch linked tasks with turns/cost
      let linkedTasksHtml = '';
      try {
        const linkedTasks = await fetchJSON('/api/manifests/' + m.id + '/tasks');
        if (linkedTasks && linkedTasks.length) {
          const statusCounts = {};
          for (const t of linkedTasks) { statusCounts[t.status] = (statusCounts[t.status] || 0) + 1; }
          const summary = Object.entries(statusCounts).map(([s,c]) => `${c} ${s}`).join(', ');
          // Shared status styling — see internal/web/ui/task-status.js.
          const statusColors = OL.TASK_STATUS_COLORS;
          const statusIcons = OL.TASK_STATUS_ICONS;
          const taskRows = linkedTasks.map(t => {
            const color = statusColors[t.status] || 'var(--text-muted)';
            const icon = statusIcons[t.status] || '&#x25CB;';
            const turnsStr = (t.total_turns || t.turns || 0) > 0 ? `${t.total_turns || t.turns} turns` : '';
            const costStr = (t.total_cost || t.cost || 0) > 0 ? `$${(t.total_cost || t.cost).toFixed(2)}` : '';
            const runsStr = t.run_count > 0 ? `${t.run_count} runs` : '';
            const lastRunStr = t.last_run_at ? `last: ${new Date(t.last_run_at).toLocaleString()}` : '';
            return `<div class="task-nav" data-tid="${t.id}" role="button" tabindex="0" style="display:flex;align-items:center;gap:8px;padding:8px 10px;border-bottom:1px solid var(--border);cursor:pointer;font-size:12px">
              <span style="color:${color};font-size:14px;min-width:16px">${icon}</span>
              <span class="session-uuid" style="font-size:11px">${esc(t.marker)}</span>
              <span style="flex:1;color:var(--text-primary)">${esc(t.title)}</span>
              <span style="color:${color};font-weight:600;text-transform:uppercase;font-size:10px;min-width:60px">${esc(t.status)}</span>
              ${runsStr ? `<span style="color:var(--text-muted);font-size:11px;min-width:45px">${runsStr}</span>` : '<span style="min-width:45px"></span>'}
              ${turnsStr ? `<span style="color:var(--text-muted);font-size:11px;min-width:55px">${turnsStr}</span>` : '<span style="min-width:55px"></span>'}
              ${costStr ? `<span style="color:var(--text-muted);font-size:11px;min-width:45px">${costStr}</span>` : '<span style="min-width:45px"></span>'}
              ${lastRunStr ? `<span style="color:var(--text-muted);font-size:10px">${lastRunStr}</span>` : ''}
            </div>`;
          }).join('');
          linkedTasksHtml = `<div style="margin-bottom:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div class="section-title">Executed Tasks <span class="sub-count">(${linkedTasks.length}) &mdash; ${summary}</span></div>
            <div class="bordered-container">${taskRows}</div>
          </div>`;
        } else {
          linkedTasksHtml = `<div style="margin-bottom:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div class="section-title">Executed Tasks</div>
            <div class="empty-placeholder">No tasks executed yet</div>
          </div>`;
        }
      } catch(e) {}

      const statusOptions = ['draft', 'open', 'closed', 'archive'];
      const statusBtnColors = {draft:'var(--yellow)',open:'var(--green)',closed:'var(--text-muted)',archive:'var(--red)'};
      const statusToggleHtml = statusOptions.map(s => {
        const active = m.status === s;
        const color = statusBtnColors[s] || 'var(--text-muted)';
        return `<button class="manifest-status-btn" data-status="${s}" style="
          padding:4px 12px;font-size:11px;font-weight:600;text-transform:uppercase;border-radius:4px;cursor:pointer;
          border:1px solid ${color};
          background:${active ? color : 'transparent'};
          color:${active ? 'var(--bg-primary)' : color};
          opacity:${active ? '1' : '0.7'};
        ">${s}</button>`;
      }).join('');

      bodyEl.innerHTML = `
        <div class="manifest-detail-view">
          <!-- BREADCRUMB -->
          <div class="breadcrumb">
            <span class="breadcrumb-link" onclick="OL.switchView('${product ? 'products' : 'manifests'}')">${esc(m.source_node ? m.source_node.substring(0,12) : 'node')}</span>
            ${product ? `<span class="breadcrumb-sep"> → </span><span class="breadcrumb-link" onclick="OL.switchView('products');setTimeout(()=>OL.loadProduct('${esc(product.id)}'),300)">${esc(product.marker)} ${esc(product.title)}</span>` : ''}
            <span class="breadcrumb-sep"> → </span>
            <span style="color:var(--text-primary)">${esc(m.marker)} ${esc(m.title)}</span>
          </div>
          <div class="manifest-meta">
            <span class="session-uuid" style="font-size:14px">${esc(m.marker)}</span>
            <span style="font-size:12px;color:var(--text-muted)">v${m.version}</span>
            <span style="font-size:12px;color:var(--text-muted)">by ${esc(m.author)}</span>
            ${jira ? `<span style="font-size:12px;color:var(--text-muted)">Jira: ${jira}</span>` : ''}
          </div>
          <!-- TOP TOOLBAR — single row for every action on the manifest detail panel -->
          <div class="toolbar-row">
            <button id="manifest-toolbar-edit" class="btn-search btn-action">&#9998; Edit</button>
            <button id="manifest-toolbar-new-task" class="btn-search btn-action">+ New Task</button>
            <button id="manifest-toolbar-link-task" class="btn-search btn-action" style="background:var(--bg-input)">+ Link Task</button>
            <button id="manifest-add-dep-btn" class="btn-search btn-action">+ Dep</button>
            ${product ? `<button id="manifest-toolbar-dag" class="btn-search btn-action" onclick="OL.showProductDiagram('${esc(product.id)}','${esc(product.title)}')">&#x25C8; Product DAG</button>` : ''}
            ${statusToggleHtml}
          </div>
          <div id="manifest-revisions-mount" style="margin-bottom:12px"></div>
          <!-- METADATA BAR -->
          <div class="stats-bar">
            <span>Tasks: <strong style="color:var(--text-primary)">${m.total_tasks || 0}</strong></span>
            <span>Turns: <strong style="color:var(--text-primary)">${m.total_turns || 0}</strong></span>
            <span>Cost: <strong style="color:var(--green)">$${(m.total_cost || 0).toFixed(2)}</strong></span>
            <span class="separator">|</span>
            <span>Product: <strong id="manifest-product-display" style="color:${product ? 'var(--accent)' : 'var(--text-muted)'};cursor:pointer" title="Click to change product">${product ? esc(product.marker) + ' ' + esc(product.title) : 'None'}</strong></span>
            <span class="separator">|</span>
            <span>Created: ${new Date(m.created_at).toLocaleString()}</span>
            <span>Updated: ${new Date(m.updated_at).toLocaleString()}</span>
          </div>
          <div id="manifest-edit-desc" class="manifest-editable md-body" style="margin-bottom:12px;padding:4px;border-radius:4px;cursor:pointer" title="Click to edit description">${m.description_html || esc(m.description) || '<span style="color:var(--text-muted);font-style:italic">No description — click to add</span>'}</div>
          <div style="margin-bottom:4px">
            <span style="font-size:12px;color:var(--text-muted);font-weight:500">Spec / Content</span>
          </div>
          <div id="manifest-content-display" class="manifest-content md-body">${m.content_html || esc(m.content)}</div>
          <div id="manifest-content-editor" style="display:none;margin-bottom:12px">
            <textarea id="manifest-content-textarea" class="conv-search" style="width:100%;min-height:300px;font-family:monospace;font-size:13px;padding:12px;resize:vertical;line-height:1.5">${esc(m.content)}</textarea>
            <div style="margin-top:6px;display:flex;gap:8px">
              <button id="manifest-content-save" class="btn-search" style="padding:4px 16px;font-size:12px">Save</button>
              <button id="manifest-content-cancel" class="btn-dismiss" style="padding:4px 12px;font-size:12px">Cancel</button>
            </div>
          </div>
          ${depsHtml}
          ${linkedIdeasHtml}
          ${linkedTasksHtml}
          <div id="manifest-knobs-mount" style="margin-top:16px"></div>
          <div id="manifest-comments-mount" style="margin-top:16px"></div>
          ${tags ? `<div style="margin-bottom:12px">${tags}</div>` : ''}
          <div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border);font-size:11px;color:var(--text-muted);display:flex;gap:16px">
            <span>Created: ${new Date(m.created_at).toLocaleString()}</span>
            <span>Updated: ${new Date(m.updated_at).toLocaleString()}</span>
            <span>ID: ${esc(m.id)}</span>
          </div>
        </div>`;

      const knobMount = document.getElementById('manifest-knobs-mount');
      if (knobMount && OL.renderKnobSection) {
        OL.renderKnobSection(knobMount, { type: 'manifest', id: m.id });
      }

      const revisionsMount = document.getElementById('manifest-revisions-mount');
      if (revisionsMount && OL.renderRevisionsSection) {
        OL.renderRevisionsSection(revisionsMount, { type: 'manifest', id: m.id });
      }

      const commentsMount = document.getElementById('manifest-comments-mount');
      if (commentsMount && OL.renderCommentsSection) {
        OL.renderCommentsSection(commentsMount, { type: 'manifest', id: m.id });
      }

      // Bind idea navigation links
      bodyEl.querySelectorAll('.idea-nav').forEach(el => {
        OL.onView(el, 'click', () => OL.goToIdea(el.dataset.iid));
      });

      // Bind task navigation links
      bodyEl.querySelectorAll('.task-nav').forEach(el => {
        OL.onView(el, 'click', () => {
          OL.switchView('tasks');
          setTimeout(() => OL.loadTaskDetail(el.dataset.tid), 300);
        });
      });

      // Bind dependency pill navigation
      bodyEl.querySelectorAll('.dep-pill-nav').forEach(el => {
        OL.onView(el, 'click', (e) => {
          e.stopPropagation();
          OL.loadManifest(el.dataset.depNavId);
        });
      });

      // Bind dependency remove buttons — single-edge DELETE via #81's
      // endpoint so the server can fire the rehab handler (#79) that
      // flips any waiting tasks newly-unblocked by this removal from
      // waiting → pending (Option B).
      bodyEl.querySelectorAll('.dep-remove').forEach(el => {
        OL.onView(el, 'click', async (e) => {
          e.stopPropagation();
          const removeId = el.dataset.depRmId;
          try {
            const res = await fetch('/api/manifests/' + m.id + '/dependencies/' + removeId, {
              method: 'DELETE'
            });
            if (!res.ok && res.status !== 204) {
              const msg = await res.text().catch(() => '');
              console.error('Remove dependency failed:', res.status, msg);
              return;
            }
            OL.loadManifest(m.id);
            OL.loadManifests();
          } catch(err) {
            console.error('Remove dependency failed:', err);
          }
        });
      });

      // Bind "Add Dependency" button + picker — single-edge POST via
      // #81's endpoint. The server runs cycle detection before the
      // insert; cycles come back as 409 and self-loops as 400. We
      // render the server error inline so the operator sees which
      // edge was refused without opening DevTools.
      const addDepBtn = document.getElementById('manifest-add-dep-btn');
      const depPicker = document.getElementById('manifest-dep-picker');
      const depSelect = document.getElementById('manifest-dep-select');
      const depError = document.getElementById('manifest-dep-error');
      const showDepError = (msg) => {
        if (!depError) return;
        depError.textContent = msg;
        depError.style.display = msg ? 'block' : 'none';
      };
      if (addDepBtn && depPicker && depSelect) {
        OL.onView(addDepBtn, 'click', () => {
          const visible = depPicker.style.display !== 'none';
          depPicker.style.display = visible ? 'none' : 'block';
          showDepError('');
          if (!visible) depSelect.focus();
        });
        OL.onView(depSelect, 'change', async () => {
          const selectedId = depSelect.value;
          if (!selectedId) return;
          showDepError('');
          try {
            const res = await fetch('/api/manifests/' + m.id + '/dependencies', {
              method: 'POST',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({depends_on_id: selectedId})
            });
            if (!res.ok) {
              // 409 = cycle, 400 = self-loop / bad body, 404 = unknown manifest
              const body = await res.json().catch(() => ({}));
              showDepError(body.error || ('Add failed: HTTP ' + res.status));
              depSelect.value = '';
              return;
            }
            OL.loadManifest(m.id);
            OL.loadManifests();
          } catch(err) {
            showDepError('Add failed: ' + (err && err.message ? err.message : err));
          }
        });
      }

      // Bind status toggle buttons
      bodyEl.querySelectorAll('.manifest-status-btn').forEach(btn => {
        OL.onView(btn, 'click', async () => {
          const newStatus = btn.dataset.status;
          if (newStatus === m.status) return;
          try {
            await fetch('/api/manifests/' + m.id, {
              method: 'PUT',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({status: newStatus})
            });
            OL.loadManifest(m.id);
            OL.loadManifests();
          } catch(e) {
            console.error('Update manifest status failed:', e);
          }
        });
      });

      // Inline edit: Title (click to edit)
      const titleSpan = document.getElementById('manifest-edit-title');
      if (titleSpan) {
        OL.onView(titleSpan, 'click', () => {
          const input = document.createElement('input');
          input.type = 'text';
          input.value = m.title;
          input.className = 'conv-search';
          input.style.cssText = 'font-size:inherit;font-weight:inherit;padding:2px 4px;width:300px';
          titleSpan.replaceWith(input);
          input.focus();
          input.select();
          const save = async () => {
            const val = input.value.trim();
            if (val && val !== m.title) {
              await fetch('/api/manifests/' + m.id, {
                method: 'PUT',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({title: val})
              });
              OL.loadManifests();
            }
            OL.loadManifest(m.id);
          };
          OL.onView(input, 'blur', save);
          OL.onView(input, 'keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); save(); } if (e.key === 'Escape') OL.loadManifest(m.id); });
        });
      }

      // Inline edit: Description (click to edit)
      const descEl = document.getElementById('manifest-edit-desc');
      if (descEl) {
        OL.onView(descEl, 'click', () => {
          const input = document.createElement('input');
          input.type = 'text';
          input.value = m.description || '';
          input.className = 'conv-search';
          input.style.cssText = 'font-size:13px;padding:4px;width:100%';
          input.placeholder = 'Enter description...';
          descEl.replaceWith(input);
          input.focus();
          const save = async () => {
            const val = input.value.trim();
            if (val !== (m.description || '')) {
              await fetch('/api/manifests/' + m.id, {
                method: 'PUT',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({description: val})
              });
              OL.loadManifests();
            }
            OL.loadManifest(m.id);
          };
          OL.onView(input, 'blur', save);
          OL.onView(input, 'keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); save(); } if (e.key === 'Escape') OL.loadManifest(m.id); });
        });
      }

      // Inline edit: Product assignment (click to show dropdown)
      const productDisplay = document.getElementById('manifest-product-display');
      if (productDisplay) {
        OL.onView(productDisplay, 'click', () => {
          const productOpts = allProducts.map(p =>
            `<option value="${esc(p.id)}"${m.project_id === p.id ? ' selected' : ''}>${esc(p.marker)} ${esc(p.title)}</option>`
          ).join('');
          const sel = document.createElement('select');
          sel.className = 'conv-filter';
          sel.style.cssText = 'font-size:12px;padding:2px 6px;font-family:var(--font-mono)';
          sel.innerHTML = `<option value="">No Product</option>${productOpts}`;
          sel.value = m.project_id || '';
          productDisplay.replaceWith(sel);
          sel.focus();
          const save = async () => {
            const newPid = sel.value;
            if (newPid !== (m.project_id || '')) {
              await fetch('/api/manifests/' + m.id, {
                method: 'PUT',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({project_id: newPid})
              });
              OL.loadManifests();
            }
            OL.loadManifest(m.id);
          };
          OL.onView(sel, 'change', save);
          OL.onView(sel, 'blur', () => OL.loadManifest(m.id));
        });
      }

      // Toolbar Edit → mount EasyMDE on the Spec/Content textarea. The
      // editor lives in #manifest-content-editor (hidden until Edit is
      // clicked); we only construct it on-demand to avoid paying the
      // setup cost for every detail render.
      const contentDisplay = document.getElementById('manifest-content-display');
      const contentEditor = document.getElementById('manifest-content-editor');
      let contentMDE = null;
      const closeContentEditor = () => {
        if (contentMDE) { contentMDE.detach(); contentMDE = null; }
        if (!contentDisplay || !contentEditor) return;
        contentEditor.style.display = 'none';
        contentDisplay.style.display = '';
      };
      const saveContent = async () => {
        const val = contentMDE ? contentMDE.value() : document.getElementById('manifest-content-textarea').value;
        await fetch('/api/manifests/' + m.id, {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({content: val})
        });
        closeContentEditor();
        OL.loadManifests();
        OL.loadManifest(m.id);
      };
      const openContentEditor = () => {
        if (!contentDisplay || !contentEditor) return;
        contentDisplay.style.display = 'none';
        contentEditor.style.display = 'block';
        if (!contentMDE) {
          contentMDE = OL.mountEditor(document.getElementById('manifest-content-textarea'), {
            placeholder: 'Manifest spec / content in markdown…',
            onSave: saveContent,
            onCancel: closeContentEditor,
          });
        }
        contentEditor.scrollIntoView({ behavior: 'smooth', block: 'center' });
        if (contentMDE && contentMDE.focus) setTimeout(() => contentMDE.focus(), 250);
      };
      if (contentDisplay && contentEditor) {
        OL.onView(document.getElementById('manifest-content-cancel'), 'click', closeContentEditor);
        OL.onView(document.getElementById('manifest-content-save'), 'click', saveContent);
      }
      const toolbarEdit = document.getElementById('manifest-toolbar-edit');
      if (toolbarEdit) OL.onView(toolbarEdit, 'click', openContentEditor);

      // Toolbar: + New Task → switch to tasks view, open the create form, prefill this manifest.
      const toolbarNewTask = document.getElementById('manifest-toolbar-new-task');
      if (toolbarNewTask) OL.onView(toolbarNewTask, 'click', () => {
        OL.switchView('tasks');
        setTimeout(() => {
          if (OL.showTaskCreateForm) OL.showTaskCreateForm();
          setTimeout(() => {
            const sel = document.getElementById('tc-manifest-id');
            if (sel) sel.value = m.id;
          }, 100);
        }, 300);
      });

      // Toolbar: + Link Task → pick an existing (standalone or differently-linked) task and
      // PUT its manifest_id to this manifest. Uses /api/tasks for the list.
      const toolbarLinkTask = document.getElementById('manifest-toolbar-link-task');
      if (toolbarLinkTask) OL.onView(toolbarLinkTask, 'click', async () => {
        try {
          const allTasks = await fetchJSON('/api/tasks');
          const candidates = (allTasks || []).filter(t => t.manifest_id !== m.id);
          if (!candidates.length) { alert('No tasks available to link'); return; }
          const pick = prompt('Marker of task to link to this manifest:\n\n' +
            candidates.slice(0, 40).map(t => `${t.marker}  ${t.title}  [${t.status}]`).join('\n'));
          if (!pick) return;
          const task = candidates.find(t => t.marker === pick.trim() || t.id === pick.trim());
          if (!task) { alert('No matching task'); return; }
          await fetchJSON('/api/tasks/' + task.id, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({manifest_id: m.id})
          });
          OL.loadManifest(m.id);
        } catch (e) { alert('Link failed: ' + e.message); }
      });

    } catch (e) {
      console.error('Load manifest failed:', e);
    }
  };

  OL.archiveManifest = async function(id) {
    await fetchJSON('/api/manifests/' + id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({status: 'archive'}) });
    OL.loadManifests();
    OL.loadManifest(id);
  };

})(window.OL);
