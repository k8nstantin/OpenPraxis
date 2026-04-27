import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Dialog, DialogClose } from './Dialog';
import { Button } from './Button';

const meta: Meta = {
  title: 'UI/Dialog',
  tags: ['autodocs'],
};
export default meta;

export const Basic: StoryObj = {
  render: () => {
    const [open, setOpen] = useState(false);
    return (
      <>
        <Button variant="primary" onClick={() => setOpen(true)}>Open dialog</Button>
        <Dialog
          open={open}
          onOpenChange={setOpen}
          title="Confirm action"
          description="This will start the task immediately."
          footer={
            <>
              <DialogClose asChild><Button>Cancel</Button></DialogClose>
              <Button variant="primary" onClick={() => setOpen(false)}>Start</Button>
            </>
          }
        >
          <p>Tasks created via this dialog fire on save and cannot be paused mid-run.</p>
        </Dialog>
      </>
    );
  },
};
