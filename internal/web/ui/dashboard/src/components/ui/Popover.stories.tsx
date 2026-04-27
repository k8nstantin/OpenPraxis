import type { Meta, StoryObj } from '@storybook/react';
import { Popover } from './Popover';
import { Button } from './Button';

const meta: Meta<typeof Popover> = { title: 'UI / Popover', component: Popover };
export default meta;

export const Default: StoryObj<typeof Popover> = {
  render: () => (
    <Popover trigger={<Button>Show popover</Button>}>
      <div style={{ padding: 12 }}>Popover content</div>
    </Popover>
  ),
};
