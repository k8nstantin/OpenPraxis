(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc;

  var chatSessions = [];
  var chatActiveSessionId = null;
  var chatModels = [];
  var chatAbortController = null;
  var chatStreaming = false;
  var chatAttachments = [];

  var providerColors = {
    anthropic: '#d97706',
    google: '#4285f4',
    openai: '#10a37f',
    ollama: '#8b5cf6',
  };

  var providerLabels = {
    anthropic: 'Anthropic',
    google: 'Google',
    openai: 'OpenAI',
    ollama: 'Ollama',
  };

  OL.loadChat = async function() {
    try {
      var results = await Promise.all([
        fetchJSON('/api/chat/sessions'),
        fetchJSON('/api/chat/models'),
      ]);
      var sessions = results[0];
      var modelData = results[1];
      chatSessions = sessions || [];
      chatModels = modelData.models || [];
      populateModelSelect();
      renderChatTabs();
      if (chatSessions.length === 0) {
        await createChatSession();
      } else if (!chatActiveSessionId || !chatSessions.find(function(s) { return s.id === chatActiveSessionId; })) {
        switchChatTab(chatSessions[0].id);
      } else {
        switchChatTab(chatActiveSessionId);
      }
    } catch (e) {
      console.error('Load chat failed:', e);
    }
  };

  function populateModelSelect() {
    var sel = document.getElementById('chat-model-select');
    if (!sel) return;

    // Group by provider
    var groups = {};
    for (var i = 0; i < chatModels.length; i++) {
      var m = chatModels[i];
      if (!groups[m.provider]) groups[m.provider] = [];
      groups[m.provider].push(m);
    }

    var html = '';
    var order = ['anthropic', 'google', 'openai', 'ollama'];
    for (var o = 0; o < order.length; o++) {
      var prov = order[o];
      var models = groups[prov];
      if (!models || models.length === 0) continue;
      var label = providerLabels[prov] || prov;
      html += '<optgroup label="' + esc(label) + '">';
      for (var j = 0; j < models.length; j++) {
        var mod = models[j];
        var cost = mod.cost_per_m_in > 0 ? ' $' + mod.cost_per_m_in + '/$' + mod.cost_per_m_out : ' free';
        html += '<option value="' + esc(mod.id) + '">' + esc(mod.name) + cost + '</option>';
      }
      html += '</optgroup>';
    }
    sel.innerHTML = html;
  }
  OL.populateModelSelect = populateModelSelect;

  // Expose chatModels for settings.js
  OL.getChatModels = function() { return chatModels; };
  OL.setChatModels = function(m) { chatModels = m; };

  function renderChatTabs() {
    var container = document.getElementById('chat-tabs');
    if (!container) return;
    container.innerHTML = chatSessions.map(function(s) {
      var active = s.id === chatActiveSessionId ? 'background:var(--bg-card-hover);color:var(--text);border-bottom:2px solid var(--accent)' : 'color:var(--text-muted)';
      var title = s.title || 'New Chat';
      var shortTitle = title.length > 25 ? title.substring(0, 25) + '...' : title;
      return '<div class="chat-tab" data-id="' + esc(s.id) + '" style="display:flex;align-items:center;gap:4px;padding:6px 12px;cursor:pointer;font-size:12px;white-space:nowrap;border-radius:4px 4px 0 0;' + active + ';flex-shrink:0;position:relative" title="' + esc(title) + '">' +
        '<span class="chat-tab-title" style="max-width:150px;overflow:hidden;text-overflow:ellipsis">' + esc(shortTitle) + '</span>' +
        '<span class="chat-tab-close" style="font-size:14px;opacity:0.5;cursor:pointer;padding:0 2px" title="Close">&times;</span>' +
      '</div>';
    }).join('');

    // Tab click handlers
    container.querySelectorAll('.chat-tab').forEach(function(tab) {
      OL.onView(tab, 'click', function(e) {
        if (e.target.classList.contains('chat-tab-close')) return;
        switchChatTab(tab.dataset.id);
      });
      OL.onView(tab.querySelector('.chat-tab-close'), 'click', function(e) {
        e.stopPropagation();
        deleteChatSession(tab.dataset.id);
      });
      // Double-click to rename
      OL.onView(tab.querySelector('.chat-tab-title'), 'dblclick', function(e) {
        e.stopPropagation();
        var titleEl = e.target;
        var id = tab.dataset.id;
        var sess = chatSessions.find(function(s) { return s.id === id; });
        if (!sess) return;
        var input = document.createElement('input');
        input.type = 'text';
        input.value = sess.title;
        input.style.cssText = 'font-size:12px;width:120px;padding:1px 4px;border:1px solid var(--accent);border-radius:3px;background:var(--bg-input);color:var(--text)';
        titleEl.replaceWith(input);
        input.focus();
        input.select();
        var finish = async function() {
          var newTitle = input.value.trim() || sess.title;
          await fetch('/api/chat/sessions/' + id + '/title', {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({title: newTitle}),
          });
          sess.title = newTitle;
          renderChatTabs();
        };
        OL.onView(input, 'blur', finish);
        OL.onView(input, 'keydown', function(ev) { if (ev.key === 'Enter') { ev.preventDefault(); finish(); }});
      });
    });
  }

  function switchChatTab(id) {
    chatActiveSessionId = id;
    renderChatTabs();
    var sess = chatSessions.find(function(s) { return s.id === id; });
    if (!sess) return;
    // Update model/thinking selects
    var modelSel = document.getElementById('chat-model-select');
    if (modelSel) modelSel.value = sess.model;
    var thinkSel = document.getElementById('chat-thinking-select');
    if (thinkSel) thinkSel.value = sess.thinking_level || 'off';
    // Update model provider indicator
    updateModelIndicator(sess.model);
    // Update usage display
    updateChatUsage(sess);
    // Render messages
    renderChatMessages(sess.messages || []);
  }

  function updateChatUsage(sess) {
    var el = document.getElementById('chat-usage');
    if (el && sess) {
      el.textContent = (sess.tokens_in + sess.tokens_out) + ' tokens | $' + sess.cost_usd.toFixed(4);
    }
  }

  function updateModelIndicator(modelId) {
    var indicator = document.getElementById('chat-model-indicator');
    if (!indicator || !modelId) return;
    var provider = modelId.split('/')[0];
    var color = providerColors[provider] || 'var(--text-muted)';
    indicator.style.background = color;
    indicator.title = providerLabels[provider] || provider;
  }

  function renderChatMessages(messages) {
    var container = document.getElementById('chat-messages');
    if (!container) return;
    if (!messages || messages.length === 0) {
      container.innerHTML = '<div class="empty-state" style="margin:auto;text-align:center;color:var(--text-muted)"><div style="font-size:32px;margin-bottom:12px">&#x1F4AC;</div><div style="font-size:15px;font-weight:500">Start a conversation</div><div style="font-size:13px;margin-top:6px;color:var(--text-muted)">Ask about memories, manifests, tasks, or anything else</div></div>';
      return;
    }
    container.innerHTML = messages.map(function(msg, i) {
      if (msg.role === 'tool') return ''; // tool results shown inline with tool_calls
      var isUser = msg.role === 'user';
      var bgColor = isUser ? 'var(--bg-secondary)' : 'transparent';
      var label = isUser ? 'You' : 'Assistant';
      var labelColor = isUser ? 'var(--accent)' : 'var(--green)';
      var borderLeft = isUser ? '' : 'border-left:2px solid var(--green);padding-left:14px;';

      var content = formatChatContent(msg.content);

      // Show tool calls if present
      var toolCallsHtml = '';
      if (msg.tool_calls && msg.tool_calls.length > 0) {
        toolCallsHtml = '<div style="margin-top:10px;display:flex;flex-direction:column;gap:6px">' + msg.tool_calls.map(function(tc) {
          var nextMsg = messages[i + 1];
          var result = (nextMsg && nextMsg.role === 'tool' && nextMsg.tool_call_id === tc.id) ? nextMsg.content : '';
          return '<details style="background:var(--bg-secondary);border-radius:6px;border-left:3px solid var(--yellow);font-size:12px">' +
            '<summary style="padding:8px 12px;cursor:pointer;display:flex;align-items:center;gap:6px">' +
              '<span style="color:var(--yellow);font-weight:600">&#x2699; ' + esc(tc.name) + '</span>' +
              '<span style="color:var(--text-muted);font-family:var(--font-mono);font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:300px">' + esc(tc.input ? tc.input.substring(0, 80) : '') + '</span>' +
            '</summary>' +
            (result ? '<div style="padding:8px 12px;border-top:1px solid var(--border);color:var(--text-secondary);white-space:pre-wrap;font-family:var(--font-mono);font-size:11px;max-height:150px;overflow-y:auto">' + esc(result) + '</div>' : '') +
          '</details>';
        }).join('') + '</div>';
      }

      return '<div style="padding:14px 18px;border-radius:10px;background:' + bgColor + ';' + borderLeft + '">' +
        '<div style="font-size:11px;font-weight:700;color:' + labelColor + ';margin-bottom:8px;text-transform:uppercase;letter-spacing:0.5px">' + label + '</div>' +
        '<div class="chat-msg-content" style="font-size:15px;line-height:1.7;color:var(--text)">' + content + '</div>' +
        toolCallsHtml +
      '</div>';
    }).filter(Boolean).join('');
    container.scrollTop = container.scrollHeight;
  }

  function formatChatContent(text) {
    if (!text) return '';
    var html = esc(text);

    // Code blocks with language label
    html = html.replace(/```(\w*)\n([\s\S]*?)```/g, function(match, lang, code) {
      var langLabel = lang ? '<div style="font-size:10px;color:var(--text-muted);padding:4px 12px 0;text-transform:uppercase;letter-spacing:0.5px">' + lang + '</div>' : '';
      return '<div style="background:var(--bg-secondary);border-radius:8px;margin:10px 0;overflow:hidden;border:1px solid var(--border)">' + langLabel + '<pre style="padding:10px 14px;overflow-x:auto;font-size:13px;line-height:1.5;margin:0"><code>' + code + '</code></pre></div>';
    });

    // Inline code
    html = html.replace(/`([^`]+)`/g, '<code style="background:var(--bg-secondary);padding:2px 6px;border-radius:4px;font-size:13px;border:1px solid var(--border)">$1</code>');

    // Headers (## and ###)
    html = html.replace(/^### (.+)$/gm, '<div style="font-size:14px;font-weight:700;margin:12px 0 6px;color:var(--text)">$1</div>');
    html = html.replace(/^## (.+)$/gm, '<div style="font-size:15px;font-weight:700;margin:14px 0 8px;color:var(--text)">$1</div>');

    // Bullet lists
    html = html.replace(/^[*-] (.+)$/gm, '<div style="padding-left:16px;position:relative"><span style="position:absolute;left:4px;color:var(--text-muted)">&#x2022;</span>$1</div>');

    // Numbered lists
    html = html.replace(/^(\d+)\. (.+)$/gm, '<div style="padding-left:20px;position:relative"><span style="position:absolute;left:0;color:var(--text-muted);font-size:13px">$1.</span>$2</div>');

    // Bold
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    // Italic
    html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');

    // Links
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" style="color:var(--accent);text-decoration:underline">$1</a>');

    // Wrap in white-space:pre-wrap for line breaks
    return '<div style="white-space:pre-wrap;word-wrap:break-word">' + html + '</div>';
  }

  async function createChatSession() {
    try {
      var resp = await fetch('/api/chat/sessions', {method: 'POST'});
      var sess = await resp.json();
      chatSessions.unshift(sess);
      switchChatTab(sess.id);
      renderChatTabs();
    } catch (e) {
      console.error('Create session failed:', e);
    }
  }

  async function deleteChatSession(id) {
    try {
      await fetch('/api/chat/sessions/' + id, {method: 'DELETE'});
      chatSessions = chatSessions.filter(function(s) { return s.id !== id; });
      if (chatActiveSessionId === id) {
        if (chatSessions.length > 0) {
          switchChatTab(chatSessions[0].id);
        } else {
          chatActiveSessionId = null;
          await createChatSession();
        }
      }
      renderChatTabs();
    } catch (e) {}
  }

  async function sendChatMessage() {
    var input = document.getElementById('chat-input');
    var message = input.value.trim();
    if (!message || !chatActiveSessionId || chatStreaming) return;

    input.value = '';
    input.style.height = 'auto';
    chatStreaming = true;

    // Show send/abort toggle
    document.getElementById('chat-send-btn').style.display = 'none';
    document.getElementById('chat-abort-btn').style.display = '';

    // Add user message to UI immediately
    var sess = chatSessions.find(function(s) { return s.id === chatActiveSessionId; });
    if (!sess.messages) sess.messages = [];
    var userMsg = {role: 'user', content: message, attachments: chatAttachments.length > 0 ? chatAttachments.slice() : undefined};
    sess.messages.push(userMsg);
    renderChatMessages(sess.messages);

    // Prepare attachments
    var attachments = chatAttachments.map(function(a) { return {type: 'image', mime_type: a.mime_type, base64: a.base64}; });
    chatAttachments = [];
    var preview = document.getElementById('chat-attachments-preview');
    if (preview) { preview.style.display = 'none'; preview.innerHTML = ''; }

    // Stream response
    chatAbortController = new AbortController();
    try {
      var resp = await fetch('/api/chat', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({session_id: chatActiveSessionId, message: message, attachments: attachments}),
        signal: chatAbortController.signal,
      });

      var reader = resp.body.getReader();
      var decoder = new TextDecoder();
      var buffer = '';
      var assistantText = '';
      var toolCalls = [];

      // Add placeholder assistant message
      var assistantMsg = {role: 'assistant', content: ''};
      sess.messages.push(assistantMsg);

      while (true) {
        var readResult = await reader.read();
        if (readResult.done) break;
        buffer += decoder.decode(readResult.value, {stream: true});

        var lines = buffer.split('\n');
        buffer = lines.pop(); // Keep incomplete line in buffer

        for (var li = 0; li < lines.length; li++) {
          var line = lines[li];
          if (!line.startsWith('data: ')) continue;
          var data = line.substring(6);
          try {
            var chunk = JSON.parse(data);
            switch (chunk.type) {
              case 'text':
                assistantText += chunk.content;
                assistantMsg.content = assistantText;
                renderChatMessages(sess.messages);
                break;
              case 'thinking':
                // Show thinking inline (greyed out)
                assistantMsg.content = assistantText + '\n[thinking: ' + (chunk.content || '').substring(0, 100) + '...]';
                renderChatMessages(sess.messages);
                break;
              case 'tool_call':
                if (chunk.tool_call) {
                  toolCalls.push(chunk.tool_call);
                  if (!assistantMsg.tool_calls) assistantMsg.tool_calls = [];
                  assistantMsg.tool_calls.push(chunk.tool_call);
                  renderChatMessages(sess.messages);
                }
                break;
              case 'tool_result':
                if (chunk.tool_result) {
                  sess.messages.push({role: 'tool', content: chunk.tool_result.result, tool_call_id: chunk.tool_result.id});
                  renderChatMessages(sess.messages);
                }
                break;
              case 'done':
                assistantMsg.content = assistantText;
                if (chunk.usage) {
                  sess.tokens_in = (sess.tokens_in || 0) + chunk.usage.input_tokens;
                  sess.tokens_out = (sess.tokens_out || 0) + chunk.usage.output_tokens;
                }
                if (chunk.cost) {
                  sess.cost_usd = (sess.cost_usd || 0) + chunk.cost;
                }
                updateChatUsage(sess);
                break;
              case 'error':
                assistantMsg.content = assistantText + '\n\n[Error: ' + (chunk.error || 'Unknown') + ']';
                renderChatMessages(sess.messages);
                break;
            }
          } catch (e) {}
        }
      }

      // Final render
      assistantMsg.content = assistantText;
      renderChatMessages(sess.messages);

      // Auto-update title if this was the first message
      if (sess.title === 'New Chat' && message.length > 0) {
        sess.title = message.length > 50 ? message.substring(0, 50) + '...' : message;
        renderChatTabs();
      }

    } catch (e) {
      if (e.name !== 'AbortError') {
        console.error('Chat stream failed:', e);
        sess.messages.push({role: 'assistant', content: '[Connection error: ' + e.message + ']'});
        renderChatMessages(sess.messages);
      }
    }

    chatStreaming = false;
    chatAbortController = null;
    document.getElementById('chat-send-btn').style.display = '';
    document.getElementById('chat-abort-btn').style.display = 'none';
  }

  function abortChat() {
    if (chatAbortController) {
      chatAbortController.abort();
    }
  }

  function fileToBase64(file) {
    return new Promise(function(resolve) {
      var reader = new FileReader();
      reader.onload = function() { resolve(reader.result.split(',')[1]); };
      reader.readAsDataURL(file);
    });
  }

  function renderAttachmentPreviews() {
    var container = document.getElementById('chat-attachments-preview');
    if (!container) return;
    if (chatAttachments.length === 0) {
      container.style.display = 'none';
      return;
    }
    container.style.display = 'flex';
    container.innerHTML = chatAttachments.map(function(a, i) {
      return '<div style="position:relative;width:48px;height:48px;border-radius:6px;overflow:hidden;border:1px solid var(--border)">' +
        '<img src="data:' + esc(a.mime_type) + ';base64,' + a.base64 + '" style="width:100%;height:100%;object-fit:cover" />' +
        '<span class="chat-remove-attach" data-idx="' + i + '" style="position:absolute;top:-2px;right:2px;cursor:pointer;font-size:14px;color:var(--red,#e74c3c)">&times;</span>' +
      '</div>';
    }).join('');
    container.querySelectorAll('.chat-remove-attach').forEach(function(btn) {
      OL.onView(btn, 'click', function() {
        chatAttachments.splice(parseInt(btn.dataset.idx), 1);
        renderAttachmentPreviews();
      });
    });
  }

  // Chat event handlers (wired once on DOMContentLoaded)
  document.addEventListener('DOMContentLoaded', function() {
    var sendBtn = document.getElementById('chat-send-btn');
    var abortBtn = document.getElementById('chat-abort-btn');
    var newTabBtn = document.getElementById('chat-new-tab');
    var chatInput = document.getElementById('chat-input');
    var modelSelect = document.getElementById('chat-model-select');
    var thinkSelect = document.getElementById('chat-thinking-select');
    var fileInput = document.getElementById('chat-file-input');

    if (sendBtn) sendBtn.addEventListener('click', sendChatMessage);
    if (abortBtn) abortBtn.addEventListener('click', abortChat);
    if (newTabBtn) newTabBtn.addEventListener('click', createChatSession);

    if (chatInput) {
      chatInput.addEventListener('keydown', function(e) {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          sendChatMessage();
        }
      });
      // Auto-resize textarea
      chatInput.addEventListener('input', function() {
        chatInput.style.height = 'auto';
        chatInput.style.height = Math.min(chatInput.scrollHeight, 150) + 'px';
      });
    }

    if (modelSelect) {
      modelSelect.addEventListener('change', async function() {
        if (!chatActiveSessionId) return;
        var model = modelSelect.value;
        await fetch('/api/chat/sessions/' + chatActiveSessionId + '/model', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({model: model}),
        });
        var sess = chatSessions.find(function(s) { return s.id === chatActiveSessionId; });
        if (sess) sess.model = model;
        updateModelIndicator(model);
      });
    }

    if (thinkSelect) {
      thinkSelect.addEventListener('change', async function() {
        if (!chatActiveSessionId) return;
        var level = thinkSelect.value;
        await fetch('/api/chat/sessions/' + chatActiveSessionId + '/thinking', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({level: level}),
        });
        var sess = chatSessions.find(function(s) { return s.id === chatActiveSessionId; });
        if (sess) sess.thinking_level = level;
      });
    }

    // File attachments
    if (fileInput) {
      fileInput.addEventListener('change', async function() {
        var files = Array.from(fileInput.files);
        for (var i = 0; i < files.length; i++) {
          var file = files[i];
          if (!file.type.startsWith('image/')) continue;
          var base64 = await fileToBase64(file);
          chatAttachments.push({mime_type: file.type, base64: base64, name: file.name});
        }
        renderAttachmentPreviews();
        fileInput.value = '';
      });
    }

    // Keyboard shortcuts
    document.addEventListener('keydown', function(e) {
      if (OL.currentView() !== 'chat') return;
      // Cmd+T — new tab
      if ((e.metaKey || e.ctrlKey) && e.key === 't') {
        e.preventDefault();
        createChatSession();
      }
      // Cmd+W — close tab
      if ((e.metaKey || e.ctrlKey) && e.key === 'w') {
        e.preventDefault();
        if (chatActiveSessionId) deleteChatSession(chatActiveSessionId);
      }
      // Cmd+1-9 — switch tab by number
      if ((e.metaKey || e.ctrlKey) && e.key >= '1' && e.key <= '9') {
        e.preventDefault();
        var idx = parseInt(e.key) - 1;
        if (idx < chatSessions.length) switchChatTab(chatSessions[idx].id);
      }
      // Cmd+Shift+] / Cmd+Shift+[ — next/prev tab
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === ']') {
        e.preventDefault();
        var idxNext = chatSessions.findIndex(function(s) { return s.id === chatActiveSessionId; });
        if (idxNext < chatSessions.length - 1) switchChatTab(chatSessions[idxNext + 1].id);
      }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === '[') {
        e.preventDefault();
        var idxPrev = chatSessions.findIndex(function(s) { return s.id === chatActiveSessionId; });
        if (idxPrev > 0) switchChatTab(chatSessions[idxPrev - 1].id);
      }
    });
  });
})(window.OL);
