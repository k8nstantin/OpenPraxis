import type { Meta, StoryObj } from '@storybook/react';
import { IconButton } from './IconButton';

const meta: Meta<typeof IconButton> = {
  title: 'UI/IconButton',
  component: IconButton,
  args: { 'aria-label': 'Settings', icon: <span aria-hidden>⚙</span> },
  tags: ['autodocs'],
};
export default meta;

export const Default: StoryObj<typeof IconButton> = {};
export const Primary: StoryObj<typeof IconButton> = { args: { variant: 'primary' } };
