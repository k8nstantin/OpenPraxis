// OpenLoom — Reusable tree expand/collapse component
// Replaces 6+ duplicate toggle implementations across views.

(function(OL) {
  'use strict';

  // Wire expand/collapse toggles for a tree level.
  // container: parent DOM element
  // attrName:  full data attribute name, e.g. "data-amn-peer"
  //
  // Expects HTML structure:
  //   <div ... data-amn-peer="0"> <span class="tree-arrow">▼</span> ... </div>
  //   <div ... data-amn-peer-children="0"> ... </div>
  //
  // Clicking the header toggles display of the children container
  // and flips the arrow between ▼ (expanded) and ▶ (collapsed).
  OL.wireTreeToggles = function(container, attrName) {
    container.querySelectorAll('[' + attrName + ']').forEach(function(node) {
      node.addEventListener('click', function(e) {
        // Don't toggle if user clicked a button inside the header
        if (e.target.closest('button')) return;
        var idx = node.getAttribute(attrName);
        var children = container.querySelector('[' + attrName + '-children="' + idx + '"]');
        if (!children) return;
        var arrow = node.querySelector('.tree-arrow');
        if (!arrow) return;
        var hidden = children.style.display === 'none';
        children.style.display = hidden ? '' : 'none';
        arrow.innerHTML = hidden ? '\u25BC' : '\u25B6';
      });
    });
  };

  // Render a standard peer-level tree with expand/collapse.
  // peerGroups: array of { peer_id, count, ... }
  // options:
  //   prefix:          data-attribute prefix (e.g. "mem" → data-mem-peer)
  //   renderChildren:  function(peerGroup, peerIndex) → HTML string for peer's children
  //   emptyMessage:    message when no peer groups (optional)
  OL.renderPeerTree = function(container, peerGroups, options) {
    var esc = OL.esc;
    var prefix = options.prefix;

    if (!peerGroups || !peerGroups.length) {
      container.innerHTML = '<div class="empty-state">' + (options.emptyMessage || 'No data') + '</div>';
      return;
    }

    var html = '';
    for (var pi = 0; pi < peerGroups.length; pi++) {
      var pg = peerGroups[pi];
      html += '<div class="tree-node peer-header clickable" data-' + prefix + '-peer="' + pi + '">';
      html += '<span class="tree-arrow">\u25BC</span>';
      html += '<span class="status-dot green"></span>';
      html += '<span>' + esc(pg.peer_id) + '</span>';
      html += '<span class="count">' + pg.count + '</span>';
      html += '</div>';
      html += '<div class="peer-children" data-' + prefix + '-peer-children="' + pi + '">';
      html += options.renderChildren(pg, pi);
      html += '</div>';
    }

    container.innerHTML = html;
    OL.wireTreeToggles(container, 'data-' + prefix + '-peer');

    if (options.afterRender) options.afterRender(container);
  };

})(window.OL);
