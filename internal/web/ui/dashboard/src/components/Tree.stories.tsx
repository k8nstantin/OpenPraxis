import type { Meta, StoryObj } from '@storybook/react';
import { TreeRow } from './Tree';

interface FsNode { name: string; children?: FsNode[]; }

const root: FsNode = {
  name: 'src',
  children: [
    { name: 'components', children: [{ name: 'ui', children: [{ name: 'Button.tsx' }] }] },
    { name: 'lib', children: [{ name: 'api.ts' }] },
    { name: 'main.tsx' },
  ],
};

const meta: Meta = { title: 'Components/Tree', tags: ['autodocs'] };
export default meta;

export const Basic: StoryObj = {
  render: () => (
    <TreeRow<FsNode>
      node={root}
      level={0}
      selected={false}
      initiallyExpanded
      rowKey={(n) => n.name}
      label={(n) => <span>{n.name}</span>}
      children={(n) => n.children ?? []}
    />
  ),
};
