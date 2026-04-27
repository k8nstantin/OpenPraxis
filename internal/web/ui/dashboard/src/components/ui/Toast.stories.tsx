import type { Meta, StoryObj } from '@storybook/react';
import { Toaster, toast } from './Toast';
import { Button } from './Button';

const meta: Meta = { title: 'UI/Toast', tags: ['autodocs'] };
export default meta;

export const Basic: StoryObj = {
  render: () => (
    <div>
      <Toaster />
      <div style={{ display: 'flex', gap: 8 }}>
        <Button variant="primary" onClick={() => toast.success('Saved')}>Success</Button>
        <Button onClick={() => toast.error('HTTP 500: server error')}>Error</Button>
        <Button onClick={() => toast.message('Heads-up')}>Neutral</Button>
      </div>
    </div>
  ),
};
