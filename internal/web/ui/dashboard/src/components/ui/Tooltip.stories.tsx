import type { Meta, StoryObj } from '@storybook/react';
import { Tooltip } from './Tooltip';
import { Button } from './Button';

const meta: Meta = { title: 'UI/Tooltip', tags: ['autodocs'] };
export default meta;

export const Basic: StoryObj = {
  render: () => (
    <Tooltip content="Saves the current draft to the server" placement="top">
      <Button>Hover me</Button>
    </Tooltip>
  ),
};
