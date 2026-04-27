import type { Meta, StoryObj } from '@storybook/react';
import { EmptyState } from './EmptyState';

const meta: Meta<typeof EmptyState> = { title: 'UI / EmptyState', component: EmptyState };
export default meta;

export const Default: StoryObj<typeof EmptyState> = {
  args: { title: 'No products yet', message: 'Create one to group your manifests.' },
};
export const Error: StoryObj<typeof EmptyState> = {
  args: { tone: 'error', title: 'Failed to load', message: 'The server returned 500. Try again.' },
};
