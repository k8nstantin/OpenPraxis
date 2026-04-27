import type { Meta, StoryObj } from '@storybook/react';
import { CommentEditor } from './CommentEditor';

const meta: Meta<typeof CommentEditor> = {
  title: 'Components/CommentEditor',
  component: CommentEditor,
  tags: ['autodocs'],
  args: { onSubmit: (body: string) => console.log('submit', body) },
};
export default meta;

export const Basic: StoryObj<typeof CommentEditor> = {};
