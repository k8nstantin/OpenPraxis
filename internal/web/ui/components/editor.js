// OpenPraxis — Unified markdown editor (EasyMDE wrapper)
//
// Single entry point for every editable surface in the dashboard.
// Replaces five different cramped textareas with a real markdown editor
// (toolbar, side-by-side preview, syntax highlight, keyboard shortcuts).
//
// Usage:
//   var editor = OL.mountEditor(textareaEl, {
//     minHeight: '40vh',
//     placeholder: 'Write markdown…',
//     onSave: function() { ... },   // Cmd/Ctrl+Enter
//     onCancel: function() { ... }, // Escape
//     compact: false,               // true = comments-style smaller toolbar
//   });
//   editor.value()       — current markdown
//   editor.detach()      — convert back to plain textarea (use on Cancel)

(function (OL) {
  'use strict';

  // The full toolbar — used on the heavyweight specs (product / manifest /
  // task / idea descriptions). The compact toolbar drops fullscreen +
  // side-by-side + guide so the comments composer doesn't dominate the
  // detail panel.
  var FULL_TOOLBAR = [
    'bold', 'italic', 'heading', '|',
    'quote', 'code', 'table', '|',
    'unordered-list', 'ordered-list', '|',
    'link', 'horizontal-rule', '|',
    'preview', 'side-by-side', 'fullscreen', '|',
    'guide',
  ];
  var COMPACT_TOOLBAR = [
    'bold', 'italic', 'code', '|',
    'quote', 'unordered-list', 'ordered-list', '|',
    'link', 'preview',
  ];

  // mountEditor swaps a textarea for an EasyMDE instance and returns a
  // small facade so call sites don't poke at the EasyMDE API directly.
  // Falls back to the bare textarea (no editor) if the EasyMDE script
  // hasn't loaded yet — keeps the dashboard usable in degraded modes.
  OL.mountEditor = function (textarea, opts) {
    opts = opts || {};
    if (!textarea) return null;
    if (typeof EasyMDE === 'undefined') {
      console.warn('OL.mountEditor: EasyMDE not loaded; using bare textarea');
      return {
        value: function () { return textarea.value; },
        detach: function () {},
        focus: function () { textarea.focus(); },
      };
    }

    // Bound the editor so it never takes over the panel. minHeight gives
    // a comfortable default, maxHeight caps growth — content beyond that
    // scrolls inside the editor pane, not the dashboard.
    // Reasonable defaults — operators can expand via the fullscreen
    // toolbar button (F11) or drag the editor pane via the splitter.
    var defaultMin = opts.compact ? '80px' : '180px';
    var defaultMax = opts.compact ? '220px' : '40vh';
    var instance = new EasyMDE({
      element: textarea,
      autofocus: opts.autofocus === true, // off by default — autofocus scrolls the page
      placeholder: opts.placeholder || '',
      spellChecker: opts.spellChecker === true, // off by default — too noisy on code/markers
      status: false,
      minHeight: opts.minHeight || defaultMin,
      maxHeight: opts.maxHeight || defaultMax,
      toolbar: opts.toolbar || (opts.compact ? COMPACT_TOOLBAR : FULL_TOOLBAR),
      sideBySideFullscreen: false,
      forceSync: true, // mirror to underlying textarea on every keystroke
      indentWithTabs: false,
      tabSize: 2,
      lineWrapping: true,
      shortcuts: {
        toggleBold: 'Cmd-B',
        toggleItalic: 'Cmd-I',
        drawLink: 'Cmd-K',
        toggleHeadingSmaller: 'Cmd-H',
        toggleSideBySide: 'F9',
        toggleFullScreen: 'F11',
        togglePreview: 'Cmd-P',
      },
      previewClass: ['editor-preview', 'comment-body'],
    });

    // Cmd/Ctrl+Enter → onSave; Escape → onCancel. We bind on the wrapping
    // EasyMDE container so the shortcut works whether the user is in the
    // editor pane, the preview pane, or has focused the toolbar.
    var wrapEl = instance.element && instance.element.nextSibling;
    var onKey = function (e) {
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
    };
    if (wrapEl && wrapEl.addEventListener) {
      wrapEl.addEventListener('keydown', onKey, true);
    }
    // Also bind to the underlying CodeMirror so the shortcut fires from
    // inside the editor pane (which is what users will hit 99% of the time).
    if (instance.codemirror && instance.codemirror.on) {
      instance.codemirror.on('keydown', function (_cm, e) { onKey(e); });
    }

    return {
      value: function () { return instance.value(); },
      setValue: function (v) { instance.value(v); },
      focus: function () { instance.codemirror && instance.codemirror.focus(); },
      detach: function () {
        try { instance.toTextArea(); } catch (_) {}
      },
      instance: instance,
    };
  };
})(window.OL);
