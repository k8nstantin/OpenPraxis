(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc;

  // --- Settings ---
  OL.loadSettings = async function() {
    // Show MCP config snippet
    const snippet = document.getElementById('mcp-config-snippet');
    if (snippet) {
      snippet.textContent = JSON.stringify({
        "mcpServers": {
          "openpraxis": {
            "type": "http",
            "url": "http://127.0.0.1:8765/mcp"
          }
        }
      }, null, 2);
    }

    // Load profile
    try {
      const profile = await fetchJSON('/api/settings/profile');
      document.getElementById('profile-uuid').textContent = profile.uuid || '';
      document.getElementById('profile-name').value = profile.display_name || '';
      document.getElementById('profile-email').value = profile.email || '';
      document.getElementById('profile-avatar').value = profile.avatar || '';
      updateProfilePreview(profile);
    } catch (e) {}

    // Emoji picker
    const emojis = [
      '\u{1F600}','\u{1F60E}','\u{1F916}','\u{1F47B}','\u{1F98A}','\u{1F43A}','\u{1F981}','\u{1F42F}','\u{1F43B}','\u{1F43C}',
      '\u{1F428}','\u{1F438}','\u{1F419}','\u{1F989}','\u{1F985}','\u{1F41D}','\u{1F98B}','\u{1F432}','\u{1F525}','\u26A1',
      '\u{1F30A}','\u{1F308}','\u{1F48E}','\u{1F680}','\u{1F6F8}','\u{1F3AF}','\u{1F3AE}','\u{1F9E0}','\u{1F4BB}','\u{1F52E}',
      '\u2B50','\u{1F319}','\u2600\uFE0F','\u{1F340}','\u{1F33A}','\u{1F3D4}\uFE0F','\u{1F30D}','\u{1F3B5}','\u{1F3A8}','\u{1F6E1}\uFE0F',
      '\u2694\uFE0F','\u{1F531}','\u{1F451}','\u{1F3A9}','\u{1F9CA}','\u{1F300}','\u{1F49C}','\u{1F499}','\u{1F49A}','\u{1F9E1}',
    ];
    const pickerEl = document.getElementById('emoji-picker');
    const avatarInput = document.getElementById('profile-avatar');
    const previewSm = document.getElementById('profile-avatar-preview-sm');
    pickerEl.innerHTML = emojis.map(e =>
      `<span class="emoji-btn" style="font-size:22px;cursor:pointer;padding:4px 6px;border-radius:4px;transition:background 0.1s">${e}</span>`
    ).join('');
    pickerEl.querySelectorAll('.emoji-btn').forEach(btn => {
      OL.onView(btn, 'click', () => {
        avatarInput.value = btn.textContent;
        previewSm.textContent = btn.textContent;
        updateProfilePreview({display_name: document.getElementById('profile-name').value, email: document.getElementById('profile-email').value, avatar: btn.textContent});
      });
      OL.onView(btn, 'mouseenter', () => btn.style.background = 'var(--bg-card-hover)');
      OL.onView(btn, 'mouseleave', () => btn.style.background = '');
    });
    OL.onView(avatarInput, 'input', () => {
      previewSm.textContent = avatarInput.value.startsWith('http') ? '' : avatarInput.value;
    });
    previewSm.textContent = avatarInput.value.startsWith('http') ? '' : avatarInput.value;

    // Wire save button
    const saveBtn = document.getElementById('profile-save-btn');
    saveBtn.onclick = async () => {
      const body = {
        display_name: document.getElementById('profile-name').value.trim(),
        email: document.getElementById('profile-email').value.trim(),
        avatar: document.getElementById('profile-avatar').value.trim(),
      };
      try {
        await fetch('/api/settings/profile', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify(body)
        });
        const status = document.getElementById('profile-save-status');
        status.textContent = 'Saved';
        setTimeout(() => status.textContent = '', 2000);
        updateProfilePreview(body);
      } catch (e) {
        console.error('Save profile failed:', e);
      }
    };

    // Load chat provider settings
    try {
      const chatSettings = await fetchJSON('/api/settings/chat');
      renderChatProviderSettings(chatSettings);
    } catch (e) {
      console.error('Load chat settings failed:', e);
    }

    // Load detected agents
    try {
      const agents = await fetchJSON('/api/settings/agents');
      renderAgentSettings(agents || []);
    } catch (e) {
      console.error('Load settings failed:', e);
    }
  };

  function renderChatProviderSettings(settings) {
    if (!settings || !settings.providers) return;

    const providers = settings.providers;

    // Update status indicators
    for (const [name, info] of Object.entries(providers)) {
      const statusEl = document.getElementById('status-' + name);
      if (!statusEl) continue;

      if (info.has_key) {
        statusEl.innerHTML = '&#x2705;'; // green check
        statusEl.title = info.from_env ? 'Key from environment variable' : 'Key configured';
      } else if (name === 'ollama') {
        statusEl.innerHTML = '&#x25CB;'; // circle
        statusEl.title = 'Test connection to verify';
      } else {
        statusEl.innerHTML = '&#x274C;'; // red X
        statusEl.title = 'No API key configured';
      }

      // Show placeholder hint for env-sourced keys
      const input = document.getElementById('key-' + name);
      if (input && info.from_env && name !== 'ollama') {
        input.placeholder = 'Using ' + (info.env_var || 'env var') + ' (set to override)';
      }
      if (input && name === 'ollama' && info.host) {
        input.value = info.host;
      }
    }

    // Wire test buttons
    document.querySelectorAll('.provider-test-btn').forEach(btn => {
      btn.onclick = async () => {
        const provider = btn.dataset.provider;
        const input = document.getElementById('key-' + provider);
        const statusEl = document.getElementById('status-' + provider);
        btn.disabled = true;
        btn.textContent = '...';
        statusEl.innerHTML = '&#x23F3;'; // hourglass
        statusEl.title = 'Testing...';

        try {
          const body = {provider};
          if (provider === 'ollama') {
            body.host = input.value || 'http://localhost:11434';
          } else {
            body.api_key = input.value;
          }
          const resp = await fetch('/api/settings/chat/test', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(body),
          });
          const result = await resp.json();
          if (result.valid) {
            statusEl.innerHTML = '&#x2705;';
            statusEl.title = 'Connection valid';
          } else {
            statusEl.innerHTML = '&#x274C;';
            statusEl.title = result.error || 'Test failed';
          }
        } catch (e) {
          statusEl.innerHTML = '&#x274C;';
          statusEl.title = 'Test failed: ' + e.message;
        }
        btn.disabled = false;
        btn.textContent = 'Test';
      };
    });

    // Wire save buttons
    document.querySelectorAll('.provider-save-btn').forEach(btn => {
      btn.onclick = async () => {
        const provider = btn.dataset.provider;
        const input = document.getElementById('key-' + provider);
        if (!input.value) return;

        btn.disabled = true;
        btn.textContent = '...';

        try {
          const body = {};
          body[provider + '_key'] = input.value;
          await fetch('/api/settings/chat', {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(body),
          });
          btn.textContent = 'Saved';
          setTimeout(() => { btn.textContent = 'Save'; }, 1500);

          // Refresh models in chat view
          const modelData = await fetchJSON('/api/chat/models');
          OL.setChatModels(modelData.models || []);
          OL.populateModelSelect();

          // Re-fetch settings to update status
          const chatSettings = await fetchJSON('/api/settings/chat');
          renderChatProviderSettings(chatSettings);
        } catch (e) {
          btn.textContent = 'Error';
          setTimeout(() => { btn.textContent = 'Save'; }, 1500);
        }
        btn.disabled = false;
      };
    });
  }

  function updateProfilePreview(profile) {
    const avatarEl = document.getElementById('profile-avatar-display');
    const nameEl = document.getElementById('profile-display-preview');
    const emailEl = document.getElementById('profile-email-preview');

    const avatar = profile.avatar || profile.display_name?.charAt(0)?.toUpperCase() || '?';
    if (avatar.startsWith('http')) {
      avatarEl.innerHTML = `<img src="${esc(avatar)}" style="width:56px;height:56px;border-radius:50%;object-fit:cover" />`;
    } else {
      avatarEl.textContent = avatar;
    }
    nameEl.textContent = profile.display_name || 'Unnamed Node';
    emailEl.textContent = profile.email || '';
  }

  function renderAgentSettings(agents) {
    const el = document.getElementById('settings-agents');
    if (!agents.length) {
      el.innerHTML = '<div class="empty-state">No agents detected</div>';
      return;
    }

    el.innerHTML = agents.map(a => {
      let statusClass = 'not-installed';
      let statusText = 'Not installed';
      let button = '';

      if (a.installed && a.connected) {
        statusClass = 'connected';
        statusText = 'Connected';
        button = `<button class="btn-disconnect" onclick="OL.disconnectAgent('${esc(a.id)}')">Disconnect</button>`;
      } else if (a.installed && !a.connected) {
        statusClass = 'disconnected';
        statusText = 'Not connected';
        button = `<button class="btn-connect" onclick="OL.connectAgent('${esc(a.id)}')">Connect</button>`;
      } else {
        statusText = 'Not installed';
      }

      return `<div class="agent-card">
        <div class="agent-card-info">
          <span class="status-dot ${a.connected ? 'green' : a.installed ? 'yellow' : 'red'}"></span>
          <div>
            <div class="agent-card-name">${esc(a.name)}</div>
            <div class="agent-card-status ${statusClass}">${statusText}</div>
            <div class="agent-card-path">${esc(a.config_path)}</div>
          </div>
        </div>
        ${button}
      </div>`;
    }).join('');
  }

  OL.connectAgent = async function(id) {
    await fetch('/api/settings/agents/' + id + '/connect', {method: 'POST'});
    OL.loadSettings();
  };

  OL.disconnectAgent = async function(id) {
    await fetch('/api/settings/agents/' + id + '/disconnect', {method: 'POST'});
    OL.loadSettings();
  };
})(window.OL);
