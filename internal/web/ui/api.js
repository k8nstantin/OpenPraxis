// OpenLoom — Shared utilities and API layer
// All modules share the window.OL namespace.

window.OL = {};

(function(OL) {
  'use strict';

  OL.fetchJSON = async function(url, opts) {
    const resp = await fetch(url, opts);
    if (!resp.ok && resp.status !== 200) {
      const text = await resp.text();
      throw new Error(text || resp.statusText);
    }
    const text = await resp.text();
    if (!text) return null;
    return JSON.parse(text);
  };

  OL.setText = function(id, text) {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
  };

  OL.esc = function(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  };

  OL.timeAgo = function(iso) {
    if (!iso) return '';
    const diff = Date.now() - new Date(iso).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return mins + 'm ago';
    const hours = Math.floor(mins / 60);
    if (hours < 24) return hours + 'h ago';
    return Math.floor(hours / 24) + 'd ago';
  };

  OL.formatTime = function(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleTimeString('en-US', {hour: '2-digit', minute: '2-digit'});
  };

  OL.formatDate = function(dateStr) {
    const d = new Date(dateStr);
    const today = new Date();
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);
    if (dateStr === today.toISOString().substring(0, 10)) return 'Today';
    if (dateStr === yesterday.toISOString().substring(0, 10)) return 'Yesterday';
    return d.toLocaleDateString('en-US', {weekday: 'long', month: 'short', day: 'numeric'});
  };

  OL.formatDuration = function(ms) {
    if (ms < 1000) return ms + 'ms';
    const s = Math.floor(ms / 1000);
    if (s < 60) return s + 's';
    const m = Math.floor(s / 60);
    const rs = s % 60;
    if (m < 60) return m + 'm ' + rs + 's';
    const h = Math.floor(m / 60);
    const rm = m % 60;
    return h + 'h ' + rm + 'm';
  };

  OL.formatModel = function(model) {
    if (!model) return '';
    const parts = model.split('-');
    if (parts[0] === 'claude' && parts.length >= 3) {
      const family = parts[1].charAt(0).toUpperCase() + parts[1].slice(1);
      const version = parts.slice(2).join('.');
      return 'Claude ' + family + ' ' + version;
    }
    if (model.startsWith('gpt')) return model.toUpperCase();
    if (model.startsWith('gemini')) {
      return model.split('-').map(function(p) { return p.charAt(0).toUpperCase() + p.slice(1); }).join(' ');
    }
    return model;
  };

  OL.copy = function(text) {
    navigator.clipboard.writeText(text).then(function() {
      var toast = document.createElement('div');
      toast.className = 'copy-toast';
      toast.textContent = 'Copied!';
      document.body.appendChild(toast);
      setTimeout(function() { toast.remove(); }, 1500);
    });
  };

})(window.OL);
