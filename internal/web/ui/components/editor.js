// OpenPraxis — Markdown editor wrapper using <markdown-toolbar>
// (GitHub's @github/markdown-toolbar-element). Plain textarea
// underneath — it's how every chat / issue tracker on the planet
// renders its composer. No CodeMirror to fight, no library theme to
// override, all the keyboard affordances and cursor-aware syntax
// injection for free.
//
// The web component itself is registered by /vendor/markdown-toolbar-element.js
// (loaded as type="module" in index.html). This file just provides the
// OL.mountEditor facade so call sites don't change.
//
// Usage:
//   var editor = OL.mountEditor(textarea, {
//     compact: false,    // compact = comments composer, smaller toolbar
//     onSave: fn,        // Cmd/Ctrl+Enter
//     onCancel: fn,      // Escape
//   });
//   editor.value()       — current markdown
//   editor.setValue(s)   — replace content
//   editor.focus()
//   editor.detach()      — remove the toolbar wrapper, restore plain textarea

(function (OL) {
  'use strict';

  // Generated id used by <markdown-toolbar for="..."> to associate
  // the toolbar with its target textarea.
  var _idSeq = 0;
  function nextId(prefix) { _idSeq += 1; return prefix + '-' + _idSeq; }

  // Toolbar action set. Each item is either a button spec or a
  // separator '|'. The button spec is { tag, title, icon } where
  // tag is the markdown-toolbar element name (e.g. 'md-bold') and
  // icon is an inline SVG or unicode glyph.
  // Reference: https://github.com/github/markdown-toolbar-element
  // One toolbar, used everywhere. Comments composer and descriptions
  // get the same buttons in the same order — only the editor min-height
  // differs (comments composer is smaller so it doesn't dominate the
  // panel).
  var ACTIONS = [
    { tag: 'md-header', title: 'Heading', glyph: 'H' },
    { tag: 'md-bold', title: 'Bold (Cmd+B)', glyph: 'B', bold: true },
    { tag: 'md-italic', title: 'Italic (Cmd+I)', glyph: 'I', italic: true },
    '|',
    { tag: 'md-quote', title: 'Quote', glyph: '❝' },
    { tag: 'md-code', title: 'Code (Cmd+E)', glyph: '<>' },
    { tag: 'md-link', title: 'Link (Cmd+K)', glyph: '🔗' },
    '|',
    { tag: 'md-unordered-list', title: 'Bullet list', glyph: '• —' },
    { tag: 'md-ordered-list', title: 'Numbered list', glyph: '1.' },
    { tag: 'md-task-list', title: 'Task list', glyph: '☐' },
  ];

  function renderToolbar(forID, actions) {
    var inner = actions.map(function (a) {
      if (a === '|') return '<span class="ol-md-sep"></span>';
      var styleAttr = '';
      if (a.bold) styleAttr = ' style="font-weight:700"';
      else if (a.italic) styleAttr = ' style="font-style:italic"';
      return '<' + a.tag + '><button type="button" class="ol-md-btn" title="' + a.title + '" tabindex="-1"' + styleAttr + '>' + a.glyph + '</button></' + a.tag + '>';
    }).join('');
    return '<markdown-toolbar for="' + forID + '" class="ol-md-toolbar">' + inner + '</markdown-toolbar>';
  }

  OL.mountEditor = function (textarea, opts) {
    opts = opts || {};
    if (!textarea) return null;

    // If the textarea has no id, give it one so <markdown-toolbar for=>
    // can target it.
    if (!textarea.id) textarea.id = nextId('ol-md-textarea');

    // Mark the textarea so our CSS can theme it consistently across
    // the dashboard regardless of the call site's own classes.
    textarea.classList.add('ol-md-textarea');
    if (opts.compact) textarea.classList.add('ol-md-textarea-compact');

    // Build wrapper: toolbar above + textarea below.
    var wrapper = document.createElement('div');
    wrapper.className = 'ol-md-wrapper' + (opts.compact ? ' ol-md-wrapper-compact' : '');
    var toolbarHTML = renderToolbar(textarea.id, ACTIONS);
    wrapper.innerHTML = toolbarHTML;

    // Insert the wrapper before the textarea, then move the textarea
    // into the wrapper. This keeps the textarea's content + listeners
    // intact (no re-creation) while putting the toolbar above it.
    var parent = textarea.parentNode;
    if (parent) {
      parent.insertBefore(wrapper, textarea);
      wrapper.appendChild(textarea);
    }

    // Cmd/Ctrl+Enter saves; Escape cancels. Bound on the textarea so
    // the shortcut fires from inside the editor.
    function onKey(e) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
        if (typeof opts.onSave === 'function') {
          e.preventDefault();
          opts.onSave();
        }
      } else if (e.key === 'Escape') {
        if (typeof opts.onCancel === 'function') {
          e.preventDefault();
          opts.onCancel();
        }
      }
    }
    textarea.addEventListener('keydown', onKey);

    return {
      value: function () { return textarea.value; },
      setValue: function (v) { textarea.value = v; },
      focus: function () { textarea.focus(); },
      detach: function () {
        textarea.removeEventListener('keydown', onKey);
        textarea.classList.remove('ol-md-textarea', 'ol-md-textarea-compact');
        // Move textarea back out of the wrapper, then drop the wrapper.
        if (wrapper.parentNode) {
          wrapper.parentNode.insertBefore(textarea, wrapper);
          wrapper.remove();
        }
      },
      element: textarea,
    };
  };
})(window.OL);
