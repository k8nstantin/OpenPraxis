import type { Meta, StoryObj } from '@storybook/react';
import { MemoryRouter } from 'react-router-dom';
import { PageWrapper } from './PageWrapper';

const meta: Meta<typeof PageWrapper> = {
  title: 'Chrome / PageWrapper',
  component: PageWrapper,
  decorators: [(Story) => <MemoryRouter><Story /></MemoryRouter>],
};
export default meta;

export const Default: StoryObj<typeof PageWrapper> = {
  args: {
    title: 'Products',
    breadcrumbs: [{ label: 'Home', to: '/' }, { label: 'Products' }],
    children: <p>Tab body content lives here.</p>,
  },
};
