import type { Meta, StoryObj } from '@storybook/react';
import { MemoryRouter } from 'react-router-dom';
import { AppShell } from './AppShell';

const meta: Meta<typeof AppShell> = {
  title: 'Chrome / AppShell',
  component: AppShell,
  decorators: [(Story) => <MemoryRouter><Story /></MemoryRouter>],
};
export default meta;

export const Default: StoryObj<typeof AppShell> = {
  args: { children: <div style={{ padding: 16 }}>Page content</div> },
};
