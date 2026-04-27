import type { Meta, StoryObj } from '@storybook/react';
import { Popover } from './Popover';
import { Button } from './Button';

const meta: Meta = { title: 'UI/Popover', tags: ['autodocs'] };
export default meta;

export const Basic: StoryObj = {
  render: () => (
    <Popover trigger={<Button>Open menu</Button>}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <Button variant="ghost">Edit</Button>
        <Button variant="ghost">Duplicate</Button>
        <Button variant="danger">Delete</Button>
      </div>
    </Popover>
  ),
};
