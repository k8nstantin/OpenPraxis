(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, formatTime = OL.formatTime;

  OL.loadManifests = async function() {
    const el = document.getElementById('manifest-list');
    const searchInput = document.getElementById('manifest-search-input');
    const searchBtn = document.getElementById('manifest-search-btn');

    searchBtn.onclick = () => searchManifests(searchInput.value);
    searchInput.onkeypress = (e) => { if (e.key === 'Enter') searchManifests(searchInput.value); };

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

          return '<div class="amnesia-item ' + statusClass + ' clickable tree-leaf" data-id="' + esc(m.id) + '" style="margin-left:24px;cursor:pointer">' +
            '<div class="amnesia-header">' +
              '<span class="amnesia-status-label">' + esc(m.status) + '</span>' +
              '<span class="session-uuid">' + esc(m.marker) + '</span>' +
              (metaParts.length ? '<span style="font-size:11px;color:var(--text-muted)">' + metaParts.join(' &middot; ') + '</span>' : '') +
              '<span style="color:var(--text-muted);font-size:11px;margin-left:auto">' + formatTime(m.updated_at || '') + '</span>' +
            '</div>' +
            '<div class="amnesia-rule">' + esc(m.title) + '</div>' +
          '</div>';
        },
        leafSelector: '.tree-leaf',
        onLeafClick: function(item) { window._loadManifest(item.dataset.id); },
      });
    } catch (e) {
      console.error('Load manifests failed:', e);
    }
  };

  async function searchManifests(query) {
    if (!query.trim()) { OL.loadManifests(); return; }
    const el = document.getElementById('manifest-list');
    try {
      const resp = await fetch('/api/manifests/search', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({query})
      });
      const manifests = await resp.json();
      renderManifestList(el, manifests || []);
    } catch (e) {
      console.error('Search manifests failed:', e);
    }
  }

  function renderManifestList(el, manifests) {
    if (!manifests || !manifests.length) {
      el.innerHTML = '<div class="empty-state">No manifests found</div>';
      return;
    }
    el.innerHTML = manifests.map(m => {
      const statusClass = m.status === 'open' ? 'scope' : m.status === 'closed' ? 'type' : m.status === 'archive' ? 'type' : '';
      const jira = (m.jira_refs || []).join(', ');
      return `<div class="manifest-item clickable" data-id="${esc(m.id)}" onclick="window._loadManifest('${esc(m.id)}')">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:4px">
          <span class="session-uuid">${esc(m.marker)}</span>
          <span class="badge ${statusClass}">${esc(m.status)}</span>
          <span style="font-size:11px;color:var(--text-muted)">v${m.version}</span>
          <button class="btn-copy-sm" onclick="event.stopPropagation();window._copy('get manifest ${esc(m.marker)}')" title="Copy ref">&#x2398;</button>
        </div>
        <div class="manifest-item-title">${esc(m.title)}</div>
        <div style="font-size:12px;color:var(--text-secondary)">${esc(m.description)}</div>
        ${jira ? `<div style="font-size:11px;color:var(--accent);margin-top:4px">${esc(jira)}</div>` : ''}
      </div>`;
    }).join('');
  }

  // Promote idea or memory to manifest -- opens manifest detail panel with creation form pre-filled
  window._promoteToManifest = async function(title, description, content) {
    OL.switchView('manifests');

    // Fetch products for the dropdown
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
        `<option value="${esc(p.id)}">${esc(p.marker)} ${esc(p.title)}</option>`
      ).join('');

      bodyEl.innerHTML = `
        <div style="max-width:600px">
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Title</label>
            <input type="text" id="pm-title" class="conv-search" style="font-size:14px" value="${esc(title)}" />
          </div>
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Description</label>
            <input type="text" id="pm-description" class="conv-search" style="font-size:13px" value="${esc(description)}" />
          </div>
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Content</label>
            <textarea id="pm-content" class="conv-search" style="font-size:13px;height:200px;resize:vertical;font-family:var(--font-mono)">${esc(content)}</textarea>
          </div>
          <div style="display:flex;gap:12px;margin-bottom:16px">
            <div style="flex:1">
              <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Status</label>
              <select id="pm-status" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
                <option value="draft" selected>Draft</option>
                <option value="open">Open</option>
                <option value="closed">Closed</option>
              </select>
            </div>
            <div style="flex:1">
              <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Product</label>
              <select id="pm-product-id" class="conv-filter" style="font-size:13px;width:100%;padding:8px">
                <option value="">No Product</option>
                ${productOptions}
              </select>
            </div>
          </div>
          <div style="margin-bottom:16px">
            <label style="font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500">Jira Refs</label>
            <input type="text" id="pm-jira" class="conv-search" placeholder="ENG-1234, ENG-5678" style="font-size:13px" />
          </div>
          <div style="display:flex;gap:8px">
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
        const m = await resp.json();
        bodyEl.querySelector('#pm-status-msg').textContent = 'Created!';
        OL.loadManifests();
        setTimeout(() => window._loadManifest(m.id), 500);
      };
    }, 300);
  };

  window._loadManifest = async function(id) {
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

      titleEl.innerHTML = `<span id="manifest-edit-title" class="manifest-editable" style="cursor:pointer;border-radius:4px;padding:2px 4px" title="Click to edit title">${esc(m.title)}</span> <button class="btn-copy" onclick="window._copy('get manifest ${esc(m.marker)}')" title="Copy ref">&#x2398;</button>`;

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
          const statusColors = {running:'var(--green)',paused:'var(--yellow)',scheduled:'var(--yellow)',waiting:'var(--accent)',pending:'var(--text-muted)',completed:'var(--green)',failed:'var(--red)',cancelled:'var(--text-muted)'};
          const statusIcons = {running:'&#x25CF;',paused:'&#x23F8;',scheduled:'&#x23F0;',waiting:'&#x23F3;',pending:'&#x25CB;',completed:'&#x2713;',failed:'&#x2717;',cancelled:'&#x2015;'};
          const taskRows = linkedTasks.map(t => {
            const color = statusColors[t.status] || 'var(--text-muted)';
            const icon = statusIcons[t.status] || '&#x25CB;';
            const turnsStr = (t.total_turns || t.turns || 0) > 0 ? `${t.total_turns || t.turns} turns` : '';
            const costStr = (t.total_cost || t.cost || 0) > 0 ? `$${(t.total_cost || t.cost).toFixed(2)}` : '';
            const runsStr = t.run_count > 0 ? `${t.run_count} runs` : '';
            const lastRunStr = t.last_run_at ? `last: ${new Date(t.last_run_at).toLocaleString()}` : '';
            return `<div class="task-nav" data-tid="${t.id}" style="display:flex;align-items:center;gap:8px;padding:8px 10px;border-bottom:1px solid var(--border);cursor:pointer;font-size:12px">
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
            <div style="font-size:13px;color:var(--text-primary);margin-bottom:8px;font-weight:600">Executed Tasks <span style="font-weight:400;font-size:12px;color:var(--text-muted)">(${linkedTasks.length}) &mdash; ${summary}</span></div>
            <div style="border:1px solid var(--border);border-radius:4px;overflow:hidden">${taskRows}</div>
          </div>`;
        } else {
          linkedTasksHtml = `<div style="margin-bottom:16px;padding-top:12px;border-top:1px solid var(--border)">
            <div style="font-size:13px;color:var(--text-primary);margin-bottom:8px;font-weight:600">Executed Tasks</div>
            <div style="font-size:12px;color:var(--text-muted);padding:12px;border:1px dashed var(--border);border-radius:4px;text-align:center">
              No tasks executed yet
              <button class="btn-search manifest-create-task-btn" style="margin-left:8px;padding:4px 12px;font-size:11px">+ Create Task</button>
            </div>
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
          <div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;font-family:var(--font-mono)">
            <span style="cursor:pointer;color:var(--accent)" onclick="OL.switchView('${product ? 'products' : 'manifests'}')">${esc(m.source_node ? m.source_node.substring(0,12) : 'node')}</span>
            ${product ? `<span style="opacity:0.4"> → </span><span style="cursor:pointer;color:var(--accent)" onclick="OL.switchView('products');setTimeout(()=>window._loadProduct('${esc(product.id)}'),300)">${esc(product.marker)} ${esc(product.title)}</span>` : ''}
            <span style="opacity:0.4"> → </span>
            <span style="color:var(--text-primary)">${esc(m.marker)} ${esc(m.title)}</span>
          </div>
          <div class="manifest-meta">
            <span class="session-uuid" style="font-size:14px">${esc(m.marker)}</span>
            <div style="display:inline-flex;gap:4px;margin:0 8px">${statusToggleHtml}</div>
            <span style="font-size:12px;color:var(--text-muted)">v${m.version}</span>
            <span style="font-size:12px;color:var(--text-muted)">by ${esc(m.author)}</span>
          </div>
          <!-- METADATA BAR -->
          <div style="display:flex;gap:12px;font-size:12px;color:var(--text-muted);margin-bottom:12px;align-items:center;flex-wrap:wrap;padding:8px 12px;background:var(--bg-secondary);border:1px solid var(--border);border-radius:6px;font-family:var(--font-mono)">
            <span>Tasks: <strong style="color:var(--text-primary)">${m.total_tasks || 0}</strong></span>
            <span>Turns: <strong style="color:var(--text-primary)">${m.total_turns || 0}</strong></span>
            <span>Cost: <strong style="color:var(--green)">$${(m.total_cost || 0).toFixed(2)}</strong></span>
            <span style="opacity:0.3">|</span>
            <span>Product: <strong id="manifest-product-display" style="color:${product ? 'var(--accent)' : 'var(--text-muted)'};cursor:pointer" title="Click to change product">${product ? esc(product.marker) + ' ' + esc(product.title) : 'None'}</strong></span>
            <span style="opacity:0.3">|</span>
            <span>Created: ${new Date(m.created_at).toLocaleString()}</span>
            <span>Updated: ${new Date(m.updated_at).toLocaleString()}</span>
          </div>
          <div id="manifest-edit-desc" class="manifest-editable" style="font-size:13px;color:var(--text-secondary);margin-bottom:12px;padding:4px;border-radius:4px;cursor:pointer" title="Click to edit description">${esc(m.description) || '<span style="color:var(--text-muted);font-style:italic">No description — click to add</span>'}</div>
          ${linkedIdeasHtml}
          ${linkedTasksHtml}
          ${jira ? `<div style="margin-bottom:12px">Jira: ${jira}</div>` : ''}
          ${tags ? `<div style="margin-bottom:12px">${tags}</div>` : ''}
          <div style="margin-bottom:4px;display:flex;align-items:center;gap:8px">
            <span style="font-size:12px;color:var(--text-muted);font-weight:500">Spec / Content</span>
            <button id="manifest-edit-content-btn" class="btn-search" style="padding:2px 10px;font-size:11px">Edit</button>
          </div>
          <div id="manifest-content-display" class="manifest-content">${esc(m.content)}</div>
          <div id="manifest-content-editor" style="display:none;margin-bottom:12px">
            <textarea id="manifest-content-textarea" class="conv-search" style="width:100%;min-height:300px;font-family:monospace;font-size:13px;padding:12px;resize:vertical;line-height:1.5">${esc(m.content)}</textarea>
            <div style="margin-top:6px;display:flex;gap:8px">
              <button id="manifest-content-save" class="btn-search" style="padding:4px 16px;font-size:12px">Save</button>
              <button id="manifest-content-cancel" class="btn-dismiss" style="padding:4px 12px;font-size:12px">Cancel</button>
            </div>
          </div>
          <div style="margin-top:16px;padding-top:12px;border-top:1px solid var(--border);font-size:11px;color:var(--text-muted);display:flex;gap:16px">
            <span>Created: ${new Date(m.created_at).toLocaleString()}</span>
            <span>Updated: ${new Date(m.updated_at).toLocaleString()}</span>
            <span>ID: ${esc(m.id)}</span>
          </div>
        </div>`;

      // Bind idea navigation links
      bodyEl.querySelectorAll('.idea-nav').forEach(el => {
        el.addEventListener('click', () => window._goToIdea(el.dataset.iid));
      });

      // Bind task navigation links
      bodyEl.querySelectorAll('.task-nav').forEach(el => {
        el.addEventListener('click', () => {
          OL.switchView('tasks');
          setTimeout(() => OL.loadTaskDetail(el.dataset.tid), 300);
        });
      });

      // Bind status toggle buttons
      bodyEl.querySelectorAll('.manifest-status-btn').forEach(btn => {
        btn.addEventListener('click', async () => {
          const newStatus = btn.dataset.status;
          if (newStatus === m.status) return;
          try {
            await fetch('/api/manifests/' + m.id, {
              method: 'PUT',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({status: newStatus})
            });
            window._loadManifest(m.id);
            OL.loadManifests();
          } catch(e) {
            console.error('Update manifest status failed:', e);
          }
        });
      });

      // Inline edit: Title (click to edit)
      const titleSpan = document.getElementById('manifest-edit-title');
      if (titleSpan) {
        titleSpan.addEventListener('click', () => {
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
            window._loadManifest(m.id);
          };
          input.addEventListener('blur', save);
          input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); save(); } if (e.key === 'Escape') window._loadManifest(m.id); });
        });
      }

      // Inline edit: Description (click to edit)
      const descEl = document.getElementById('manifest-edit-desc');
      if (descEl) {
        descEl.addEventListener('click', () => {
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
            window._loadManifest(m.id);
          };
          input.addEventListener('blur', save);
          input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); save(); } if (e.key === 'Escape') window._loadManifest(m.id); });
        });
      }

      // Inline edit: Product assignment (click to show dropdown)
      const productDisplay = document.getElementById('manifest-product-display');
      if (productDisplay) {
        productDisplay.addEventListener('click', () => {
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
            window._loadManifest(m.id);
          };
          sel.addEventListener('change', save);
          sel.addEventListener('blur', () => window._loadManifest(m.id));
        });
      }

      // Inline edit: Content (toggle editor)
      const contentBtn = document.getElementById('manifest-edit-content-btn');
      const contentDisplay = document.getElementById('manifest-content-display');
      const contentEditor = document.getElementById('manifest-content-editor');
      if (contentBtn && contentDisplay && contentEditor) {
        contentBtn.addEventListener('click', () => {
          contentDisplay.style.display = 'none';
          contentBtn.style.display = 'none';
          contentEditor.style.display = 'block';
        });
        document.getElementById('manifest-content-cancel').addEventListener('click', () => {
          contentEditor.style.display = 'none';
          contentDisplay.style.display = '';
          contentBtn.style.display = '';
        });
        document.getElementById('manifest-content-save').addEventListener('click', async () => {
          const val = document.getElementById('manifest-content-textarea').value;
          await fetch('/api/manifests/' + m.id, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({content: val})
          });
          OL.loadManifests();
          window._loadManifest(m.id);
        });
      }

      // Create task button (when no tasks exist for this manifest)
      const createTaskBtn = bodyEl.querySelector('.manifest-create-task-btn');
      if (createTaskBtn) {
        createTaskBtn.addEventListener('click', () => {
          OL.switchView('tasks');
          setTimeout(() => {
            OL.showTaskCreateForm();
            setTimeout(() => {
              const sel = document.getElementById('tc-manifest-id');
              if (sel) { sel.value = m.id; }
            }, 100);
          }, 300);
        });
      }

    } catch (e) {
      console.error('Load manifest failed:', e);
    }
  };

  window._archiveManifest = async function(id) {
    await fetchJSON('/api/manifests/' + id, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({status: 'archive'}) });
    OL.loadManifests();
    window._loadManifest(id);
  };

})(window.OL);
