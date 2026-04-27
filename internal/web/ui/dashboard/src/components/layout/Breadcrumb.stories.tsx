import type { Meta, StoryObj } from '@storybook/react';
import { MemoryRouter } from 'react-router-dom';
import { Breadcrumb } from './Breadcrumb';

const meta: Meta<typeof Breadcrumb> = {
  title: 'Chrome / Breadcrumb',
  component: Breadcrumb,
  decorators: [(Story) => <MemoryRouter><Story /></MemoryRouter>],
};
export default meta;

export const Default: StoryObj<typeof Breadcrumb> = {
  args: {
    items: [
      { label: 'Home', to: '/' },
      { label: 'Products', to: '/products' },
      { label: 'OpenPraxis' },
    ],
  },
};
