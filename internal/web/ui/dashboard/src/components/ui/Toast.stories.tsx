import type { Meta, StoryObj } from '@storybook/react';
import { Toaster, toast } from './Toast';
import { Button } from './Button';

const meta: Meta<typeof Toaster> = { title: 'UI / Toast', component: Toaster };
export default meta;

export const TriggerToasts: StoryObj<typeof Toaster> = {
  render: () => (
    <>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <Button onClick={() => toast.success('Saved')}>Success</Button>
        <Button onClick={() => toast.error('Server error')}>Error</Button>
        <Button onClick={() => toast.info('Hello')}>Info</Button>
      </div>
      <Toaster />
    </>
  ),
};
