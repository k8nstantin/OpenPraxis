import type { Meta, StoryObj } from '@storybook/react';
import { EmptyState } from './EmptyState';
import { Button } from './Button';

const meta: Meta<typeof EmptyState> = {
  title: 'UI/EmptyState',
  component: EmptyState,
  tags: ['autodocs'],
};
export default meta;

export const Neutral: StoryObj<typeof EmptyState> = {
  args: {
    icon: '◇',
    title: 'No products yet',
    description: 'Create one to group your manifests.',
    action: <Button variant="primary">+ New Product</Button>,
  },
};

export const Loading: StoryObj<typeof EmptyState> = {
  args: { tone: 'loading', title: 'Loading…' },
};

export const Error: StoryObj<typeof EmptyState> = {
  args: { tone: 'error', title: 'Something went wrong', description: 'HTTP 500: server error' },
};
