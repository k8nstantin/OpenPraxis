import type { Meta, StoryObj } from '@storybook/react';
import { CommentsList } from './CommentsList';

const meta: Meta<typeof CommentsList> = {
  title: 'Components/CommentsList',
  component: CommentsList,
  tags: ['autodocs'],
};
export default meta;

export const Basic: StoryObj<typeof CommentsList> = {
  args: {
    comments: [
      { id: '1', author: 'cal', body: 'First pass merged. Picking up T1R after lunch.', created_at: '2026-04-27T14:01:00Z' },
      { id: '2', author: 'agent-claude-code', body: 'Settings catalog flag added; per-tab redirects wired.', created_at: '2026-04-27T14:42:00Z' },
    ],
  },
};
export const Empty: StoryObj<typeof CommentsList> = { args: { comments: [] } };
export const Loading: StoryObj<typeof CommentsList> = { args: { loading: true } };
