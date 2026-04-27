import type { Meta, StoryObj } from '@storybook/react';
import { IconButton } from './IconButton';

const meta: Meta<typeof IconButton> = {
  title: 'UI / IconButton',
  component: IconButton,
  args: { icon: '☰', label: 'Toggle menu' },
};
export default meta;

export const Default: StoryObj<typeof IconButton> = {};
export const Small: StoryObj<typeof IconButton> = { args: { size: 'sm' } };
export const Large: StoryObj<typeof IconButton> = { args: { size: 'lg' } };
