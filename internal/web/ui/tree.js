// OpenLoom — Reusable tree expand/collapse component
// Provides renderTree() for multi-level collapsible tree UI.
// Replaces 6+ duplicate toggle implementations across views.

(function(OL) {
  'use strict';

  var esc = OL.esc;

  // Wire expand/collapse toggles for a tree level.
  // container: parent DOM element
  // attrName:  full data attribute name, e.g. "data-amn-peer"
  //
  // Expects HTML structure:
  //   <div ... data-amn-peer="0"> <span class="tree-arrow">...</span> ... </div>
  //   <div ... data-amn-peer-children="0"> ... </div>
  //
  // Clicking the header toggles display of the children container
  // and flips the arrow between down and right.
  OL.wireTreeToggles = function(container, attrName) {
    container.querySelectorAll('[' + attrName + ']').forEach(function(node) {
      // Ensure a11y attributes on manually-built tree nodes
      if (!node.getAttribute('role')) node.setAttribute('role', 'button');
      if (!node.getAttribute('tabindex')) node.setAttribute('tabindex', '0');
      var idx = node.getAttribute(attrName);
      var children = container.querySelector('[' + attrName + '-children="' + idx + '"]');
      if (children && !node.hasAttribute('aria-expanded')) {
        node.setAttribute('aria-expanded', String(children.style.display !== 'none'));
      }

      function handleToggle(e) {
        if (e.target.closest('button')) return;
        var children = container.querySelector('[' + attrName + '-children="' + idx + '"]');
        if (!children) return;
        var arrow = node.querySelector('.tree-arrow');
        if (!arrow) return;
        var hidden = children.style.display === 'none';
        children.style.display = hidden ? '' : 'none';
        arrow.innerHTML = hidden ? '\u25BC' : '\u25B6';
        node.setAttribute('aria-expanded', String(hidden));
      }

      OL.onView(node, 'click', handleToggle);
      OL.onView(node, 'keydown', function(e) {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleToggle(e);
        }
      });
    });
  };

  // Render a multi-level collapsible tree.
  //
  // container: DOM element to render into
  // data:      array of top-level items
  // config: {
  //   prefix:       string  — unique data-attribute prefix for this tree
  //   emptyMessage: string  — shown when data is empty
  //   levels: [{
  //     label:         fn(item, idx) -> string  — display text (pre-escaped)
  //     count:         fn(item) -> number|string — count badge value
  //     children:      fn(item) -> array         — items for next level
  //     renderContent: fn(item, key) -> string   — overrides children+leaf rendering
  //     dotColor:      fn(item) -> string | string — CSS class for status-dot
  //     dotStyle:      string                    — inline style for the dot
  //     expanded:      boolean                   — default expanded (L0: true, others: false)
  //     indent:        number                    — padding-left px (default: 0 for L0, 24 for others)
  //     extra:         fn(item, idx) -> string   — extra HTML in header after label
  //     nodeAttrs:     fn(item) -> string         — extra attributes on the header div
  //     onClick:       fn(node, childrenEl, nowExpanded, item) — post-toggle callback
  //   }],
  //   renderLeaf:   fn(item, parentKey) -> string — HTML for bottom-level items
  //   leafSelector: string  — CSS selector for clickable leaf items
  //   onLeafClick:  fn(el, event) — click handler for leaf items
  //   afterRender:  fn(container) — post-render hook
  // }
  OL.renderTree = function(container, data, config) {
    var levels = config.levels || [];
    var prefix = config.prefix || ('t' + Math.random().toString(36).substr(2, 4));

    if (!data || !data.length) {
      container.innerHTML = '<div class="empty-state">' + (config.emptyMessage || 'No data') + '</div>';
      return;
    }

    // Store items keyed by level attr + key for onClick callbacks
    var _items = {};

    function buildLevel(items, levelIdx, parentKey) {
      if (levelIdx >= levels.length) {
        if (config.renderLeaf) {
          var h = '';
          for (var i = 0; i < items.length; i++) {
            h += config.renderLeaf(items[i], parentKey);
          }
          return h;
        }
        return '';
      }

      var level = levels[levelIdx];
      var expanded = levelIdx === 0 ? (level.expanded !== false) : (level.expanded === true);
      var indent = level.indent != null ? level.indent : (levelIdx > 0 ? 24 : 0);
      var attrBase = prefix + '-l' + levelIdx;
      var html = '';

      for (var i = 0; i < items.length; i++) {
        var item = items[i];
        var key = parentKey !== '' ? (parentKey + '-' + i) : String(i);

        _items[attrBase + ':' + key] = item;

        var labelStr = typeof level.label === 'function' ? level.label(item, i) : '';
        var countVal = typeof level.count === 'function' ? level.count(item) : undefined;
        var dot = typeof level.dotColor === 'function' ? level.dotColor(item) : (level.dotColor !== undefined ? level.dotColor : 'green');
        var extraStr = typeof level.extra === 'function' ? level.extra(item, i) : '';
        var nodeAttrs = typeof level.nodeAttrs === 'function' ? ' ' + level.nodeAttrs(item) : '';
        var arrow = expanded ? '\u25BC' : '\u25B6';

        // Header
        html += '<div class="tree-node' + (levelIdx === 0 ? ' peer-header' : '') + ' clickable"';
        html += ' data-' + attrBase + '="' + key + '"';
        html += nodeAttrs;
        html += ' role="button" tabindex="0" aria-expanded="' + expanded + '"';
        if (indent > 0) html += ' style="padding-left:' + indent + 'px"';
        html += '>';
        html += '<span class="tree-arrow">' + arrow + '</span>';
        if (dot) {
          html += '<span class="status-dot ' + dot + '"';
          if (level.dotStyle) html += ' style="' + level.dotStyle + '"';
          html += '></span>';
        }
        if (labelStr) html += '<span>' + labelStr + '</span>';
        if (extraStr) html += extraStr;
        if (countVal !== undefined && countVal !== '') html += '<span class="count">' + countVal + '</span>';
        html += '</div>';

        // Children container
        html += '<div';
        if (levelIdx === 0) html += ' class="peer-children"';
        html += ' data-' + attrBase + '-children="' + key + '"';
        if (!expanded) html += ' style="display:none"';
        html += '>';

        if (typeof level.renderContent === 'function') {
          html += level.renderContent(item, key);
        } else {
          var children = typeof level.children === 'function' ? level.children(item) : [];
          html += buildLevel(children, levelIdx + 1, key);
        }

        html += '</div>';
      }

      return html;
    }

    container.innerHTML = buildLevel(data, 0, '');

    // Wire toggle + onClick for each level
    for (var li = 0; li < levels.length; li++) {
      (function(levelIdx, level) {
        var attrName = 'data-' + prefix + '-l' + levelIdx;
        container.querySelectorAll('[' + attrName + ']').forEach(function(node) {
          function handleToggle(e) {
            if (e.target.closest('button')) return;
            var key = node.getAttribute(attrName);
            var childrenEl = container.querySelector('[' + attrName + '-children="' + key + '"]');
            if (!childrenEl) return;
            var arrow = node.querySelector('.tree-arrow');

            var wasHidden = childrenEl.style.display === 'none';
            childrenEl.style.display = wasHidden ? '' : 'none';
            if (arrow) arrow.innerHTML = wasHidden ? '\u25BC' : '\u25B6';
            node.setAttribute('aria-expanded', String(wasHidden));

            if (level.onClick) {
              var item = _items[prefix + '-l' + levelIdx + ':' + key];
              level.onClick(node, childrenEl, wasHidden, item);
            }
          }

          OL.onView(node, 'click', handleToggle);
          OL.onView(node, 'keydown', function(e) {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              handleToggle(e);
            }
          });
        });
      })(li, levels[li]);
    }

    // Wire leaf click handlers
    if (config.onLeafClick) {
      var selector = config.leafSelector || '.tree-leaf';
      container.querySelectorAll(selector).forEach(function(el) {
        if (!el.getAttribute('role')) el.setAttribute('role', 'button');
        if (!el.getAttribute('tabindex')) el.setAttribute('tabindex', '0');
        OL.onView(el, 'click', function(e) {
          e.stopPropagation();
          config.onLeafClick(el, e);
        });
        OL.onView(el, 'keydown', function(e) {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            e.stopPropagation();
            config.onLeafClick(el, e);
          }
        });
      });
    }

    if (config.afterRender) config.afterRender(container);
  };

})(window.OL);
