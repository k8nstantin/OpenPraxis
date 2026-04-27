import type { Meta, StoryObj } from '@storybook/react';
import { MemoryRouter } from 'react-router-dom';
import { SidebarNav } from './SidebarNav';

const meta: Meta<typeof SidebarNav> = {
  title: 'Chrome / SidebarNav',
  component: SidebarNav,
  decorators: [(Story) => <MemoryRouter initialEntries={['/products']}><Story /></MemoryRouter>],
};
export default meta;

export const Default: StoryObj<typeof SidebarNav> = {};
