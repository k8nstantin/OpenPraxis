import type { Meta, StoryObj } from '@storybook/react';
import { Tooltip, TooltipProvider } from './Tooltip';
import { Button } from './Button';

const meta: Meta<typeof Tooltip> = {
  title: 'UI / Tooltip',
  component: Tooltip,
  decorators: [(Story) => <TooltipProvider><Story /></TooltipProvider>],
};
export default meta;

export const OnButton: StoryObj<typeof Tooltip> = {
  render: () => (
    <Tooltip content="Saves changes to the server">
      <Button>Save</Button>
    </Tooltip>
  ),
};
