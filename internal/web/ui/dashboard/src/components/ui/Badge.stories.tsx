import type { Meta, StoryObj } from '@storybook/react';
import { Badge } from './Badge';

const meta: Meta<typeof Badge> = {
  title: 'UI/Badge',
  component: Badge,
  args: { children: 'open' },
  tags: ['autodocs'],
};
export default meta;

export const Neutral: StoryObj<typeof Badge> = { args: { tone: 'neutral' } };
export const Info: StoryObj<typeof Badge> = { args: { tone: 'info' } };
export const Success: StoryObj<typeof Badge> = { args: { tone: 'success' } };
export const Warning: StoryObj<typeof Badge> = { args: { tone: 'warning' } };
export const Danger: StoryObj<typeof Badge> = { args: { tone: 'danger' } };
export const Tag: StoryObj<typeof Badge> = { args: { tone: 'tag', children: 'product' } };
