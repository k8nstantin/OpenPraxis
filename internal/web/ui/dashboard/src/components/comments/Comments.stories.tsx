import type { Meta, StoryObj } from '@storybook/react';
import { CommentsList } from './CommentsList';
import { CommentEditor } from './CommentEditor';

const meta: Meta<typeof CommentsList> = {
  title: 'Cross-cutting / Comments',
  component: CommentsList,
};
export default meta;

export const List: StoryObj<typeof CommentsList> = {
  args: {
    comments: [
      { id: '1', author: 'cal@example.com', body: 'Looks great.', created_at: new Date().toISOString() },
      { id: '2', author: 'agent', body: 'Build green.', created_at: new Date().toISOString() },
    ],
  },
};

export const Empty: StoryObj<typeof CommentsList> = { args: { comments: [] } };

export const Editor: StoryObj<typeof CommentsList> = {
  render: () => <CommentEditor onSubmit={async (body) => alert('submit: ' + body)} />,
};
