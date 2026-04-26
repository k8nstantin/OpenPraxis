import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { TreeRow } from '../Tree';

interface Node {
  id: string;
  title: string;
  kids?: Node[];
}

const sample: Node = {
  id: 'root',
  title: 'Root',
  kids: [
    { id: 'a', title: 'A', kids: [{ id: 'a1', title: 'A1' }] },
    { id: 'b', title: 'B' },
  ],
};

function renderTree(opts: { initiallyExpanded?: boolean; onSelect?: (n: Node) => void } = {}) {
  return render(
    <TreeRow<Node>
      node={sample}
      level={0}
      selected={false}
      initiallyExpanded={opts.initiallyExpanded}
      label={(n) => <span>{n.title}</span>}
      rowKey={(n) => n.id}
      children={(n) => n.kids ?? []}
      onSelect={opts.onSelect}
    />,
  );
}

describe('TreeRow', () => {
  it('renders top-level only by default', () => {
    renderTree();
    expect(screen.getByText('Root')).toBeTruthy();
    expect(screen.queryByText('A')).toBeNull();
  });

  it('expands on row click', () => {
    renderTree();
    fireEvent.click(screen.getByText('Root'));
    expect(screen.getByText('A')).toBeTruthy();
    expect(screen.getByText('B')).toBeTruthy();
  });

  it('collapses again on second click', () => {
    renderTree({ initiallyExpanded: true });
    expect(screen.getByText('A')).toBeTruthy();
    fireEvent.click(screen.getByText('Root'));
    expect(screen.queryByText('A')).toBeNull();
  });

  it('fires onSelect on row click', () => {
    const onSelect = vi.fn();
    renderTree({ onSelect });
    fireEvent.click(screen.getByText('Root'));
    expect(onSelect).toHaveBeenCalledWith(sample);
  });

  it('indents children by 24px per level', () => {
    renderTree({ initiallyExpanded: true });
    const rows = screen.getAllByTestId('tree-row');
    const root = rows[0]!;
    const child = rows[1]!;
    expect(root.style.paddingLeft).toBe('0px');
    expect(child.style.paddingLeft).toBe('24px');
  });

  it('renders all 4 nodes when fully expanded', () => {
    renderTree({ initiallyExpanded: true });
    fireEvent.click(screen.getByText('A'));
    expect(screen.getAllByTestId('tree-row')).toHaveLength(4);
  });
});
