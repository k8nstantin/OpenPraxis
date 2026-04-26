import { useState, type KeyboardEvent, type MouseEvent, type ReactNode } from 'react';

// Generic recursive tree row. Replaces the levels[]/renderTree machinery
// in views/tree.js with a real component. Indent grows by 24px per
// level; chevron toggle stops propagation so the row's onClick still
// fires.

export interface TreeRowProps<T> {
  node: T;
  level: number;
  selected: boolean;
  initiallyExpanded?: boolean;
  label: (node: T) => ReactNode;
  extra?: (node: T) => ReactNode;
  children?: (node: T) => T[];
  rowKey: (node: T) => string;
  onSelect?: (node: T) => void;
  renderContent?: (node: T) => ReactNode;
  isSelected?: (node: T) => boolean;
}

const INDENT_PX = 24;

export function TreeRow<T>(props: TreeRowProps<T>) {
  const {
    node, level, selected, initiallyExpanded = false,
    label, extra, children, rowKey, onSelect, renderContent, isSelected,
  } = props;

  const kids = children?.(node) ?? [];
  const hasKids = kids.length > 0;
  const [expanded, setExpanded] = useState(initiallyExpanded);

  const toggle = (e: MouseEvent | KeyboardEvent) => {
    e.stopPropagation();
    setExpanded((v) => !v);
  };

  const onRowClick = () => {
    if (hasKids) setExpanded((v) => !v);
    onSelect?.(node);
  };

  const onRowKey = (e: KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onRowClick();
    }
  };

  return (
    <div className="tree-node">
      <div
        role="button"
        tabIndex={0}
        className={`tree-row${selected ? ' is-selected' : ''}`}
        style={{ paddingLeft: level * INDENT_PX }}
        onClick={onRowClick}
        onKeyDown={onRowKey}
        data-testid="tree-row"
      >
        <span
          className={`tree-chevron${hasKids ? '' : ' is-empty'}`}
          onClick={hasKids ? toggle : undefined}
          onKeyDown={hasKids ? toggle : undefined}
          role={hasKids ? 'button' : undefined}
          tabIndex={hasKids ? 0 : -1}
          aria-label={hasKids ? (expanded ? 'collapse' : 'expand') : undefined}
        >
          {hasKids ? (expanded ? '▾' : '▸') : ''}
        </span>
        <span className="tree-label">{label(node)}</span>
        {extra && <span className="tree-extra">{extra(node)}</span>}
      </div>
      {expanded && renderContent && (
        <div className="tree-content" style={{ paddingLeft: (level + 1) * INDENT_PX }}>
          {renderContent(node)}
        </div>
      )}
      {expanded && hasKids && (
        <div className="tree-children">
          {kids.map((child) => (
            <TreeRow<T>
              key={rowKey(child)}
              node={child}
              level={level + 1}
              selected={isSelected ? isSelected(child) : false}
              isSelected={isSelected}
              label={label}
              extra={extra}
              children={children}
              rowKey={rowKey}
              onSelect={onSelect}
              renderContent={renderContent}
            />
          ))}
        </div>
      )}
    </div>
  );
}
