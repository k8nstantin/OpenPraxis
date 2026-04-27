import type { Meta, StoryObj } from '@storybook/react';
import { CytoscapeCanvas } from './CytoscapeCanvas';

const meta: Meta<typeof CytoscapeCanvas> = {
  title: 'Cross-cutting / CytoscapeCanvas',
  component: CytoscapeCanvas,
  parameters: { layout: 'fullscreen' },
};
export default meta;

export const TwoNodes: StoryObj<typeof CytoscapeCanvas> = {
  args: {
    elements: [
      { data: { id: 'a', label: 'Alpha', type: 'product' } },
      { data: { id: 'b', label: 'Beta', type: 'manifest' } },
      { data: { id: 'a-b', source: 'a', target: 'b', edgeType: 'ownership' } },
    ],
    stylesheet: [
      { selector: 'node', style: { label: 'data(label)', 'background-color': '#1a1a2e', 'border-color': '#71717a', 'border-width': 2, color: '#e4e4e7' } },
      { selector: 'edge', style: { 'curve-style': 'straight', 'line-color': '#3b82f6', 'target-arrow-color': '#3b82f6', 'target-arrow-shape': 'triangle' } },
    ],
  },
};
