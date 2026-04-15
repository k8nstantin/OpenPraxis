// OpenLoom — View lifecycle manager
// Tracks event listeners, timeouts, and intervals per view.
// Auto-cleans all tracked resources on view switch.
// Loaded after tree.js, before views.

(function(OL) {
  'use strict';

  var _views = {};        // name -> { unmount: fn }
  var _activeView = null;
  var _tracked = {};      // name -> { listeners: [], timeouts: [], intervals: [] }

  function ensureTracking(name) {
    if (!_tracked[name]) _tracked[name] = { listeners: [], timeouts: [], intervals: [] };
    return _tracked[name];
  }

  function cleanup(name) {
    var t = _tracked[name];
    if (!t) return;
    for (var i = 0; i < t.listeners.length; i++) {
      var l = t.listeners[i];
      l.el.removeEventListener(l.event, l.handler, l.options);
    }
    t.listeners = [];
    for (var i = 0; i < t.timeouts.length; i++) clearTimeout(t.timeouts[i]);
    t.timeouts = [];
    for (var i = 0; i < t.intervals.length; i++) clearInterval(t.intervals[i]);
    t.intervals = [];
  }

  // Register a view with optional unmount handler.
  // unmount() is called before auto-cleanup on view switch.
  OL.registerView = function(name, config) {
    _views[name] = config || {};
    ensureTracking(name);
  };

  // Called by app.js on view switch: unmount + cleanup old, prepare new.
  OL._switchLifecycle = function(from, to) {
    if (from && from !== to) {
      if (_views[from] && _views[from].unmount) _views[from].unmount();
      cleanup(from);
    }
    _activeView = to;
    ensureTracking(to);
  };

  // Clean up tracked resources for the current view.
  // Call before re-rendering a view to prevent stale references.
  OL.resetViewTracking = function() {
    if (_activeView) cleanup(_activeView);
  };

  // Add an event listener tracked under the current view.
  // Automatically removed on view switch or resetViewTracking().
  OL.onView = function(element, event, handler, options) {
    if (!element) return handler;
    element.addEventListener(event, handler, options);
    if (_activeView) {
      ensureTracking(_activeView).listeners.push({
        el: element, event: event, handler: handler, options: options
      });
    }
    return handler;
  };

  // setTimeout tracked under the current view.
  // Auto-cleared on view switch.
  OL.viewTimeout = function(fn, ms) {
    var id = setTimeout(fn, ms);
    if (_activeView) ensureTracking(_activeView).timeouts.push(id);
    return id;
  };

  // setInterval tracked under the current view.
  // Auto-cleared on view switch.
  OL.viewInterval = function(fn, ms) {
    var id = setInterval(fn, ms);
    if (_activeView) ensureTracking(_activeView).intervals.push(id);
    return id;
  };

  // Get the currently active lifecycle view name.
  OL._activeLifecycleView = function() { return _activeView; };

  // Make elements keyboard-accessible: add role="button", tabindex="0",
  // and Enter/Space keydown handler that triggers click.
  // container: parent DOM element
  // selector:  CSS selector for clickable elements
  OL.a11yClick = function(container, selector) {
    container.querySelectorAll(selector).forEach(function(el) {
      if (!el.getAttribute('role')) el.setAttribute('role', 'button');
      if (!el.getAttribute('tabindex')) el.setAttribute('tabindex', '0');
      OL.onView(el, 'keydown', function(e) {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          el.click();
        }
      });
    });
  };

})(window.OL);
