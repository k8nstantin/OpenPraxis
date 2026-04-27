import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Dialog } from './Dialog';
import { Button } from './Button';

const meta: Meta<typeof Dialog> = { title: 'UI / Dialog', component: Dialog };
export default meta;

export const Default: StoryObj<typeof Dialog> = {
  render: () => {
    const [open, setOpen] = useState(false);
    return (
      <>
        <Button onClick={() => setOpen(true)}>Open dialog</Button>
        <Dialog
          open={open}
          onOpenChange={setOpen}
          title="Confirm action"
          description="This is a Radix-backed accessible dialog."
          footer={<Button variant="primary" onClick={() => setOpen(false)}>OK</Button>}
        >
          <p>Dialog body content goes here.</p>
        </Dialog>
      </>
    );
  },
};
