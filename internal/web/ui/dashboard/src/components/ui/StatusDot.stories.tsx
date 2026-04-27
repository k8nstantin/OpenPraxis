import type { Meta, StoryObj } from '@storybook/react';
import { StatusDot } from './StatusDot';

const meta: Meta<typeof StatusDot> = {
  title: 'UI/StatusDot',
  component: StatusDot,
  args: { status: 'running' },
  tags: ['autodocs'],
};
export default meta;

export const Running: StoryObj<typeof StatusDot> = { args: { status: 'running' } };
export const Completed: StoryObj<typeof StatusDot> = { args: { status: 'completed' } };
export const Failed: StoryObj<typeof StatusDot> = { args: { status: 'failed' } };
export const Paused: StoryObj<typeof StatusDot> = { args: { status: 'paused' } };
export const Cancelled: StoryObj<typeof StatusDot> = { args: { status: 'cancelled' } };
