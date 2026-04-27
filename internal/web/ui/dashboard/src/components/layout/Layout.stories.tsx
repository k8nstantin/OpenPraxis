import type { Meta, StoryObj } from '@storybook/react';
import { MemoryRouter } from 'react-router-dom';
import { Header } from './Header';
import { SidebarNav } from './SidebarNav';
import { Breadcrumb } from './Breadcrumb';
import { PageWrapper } from './PageWrapper';
import { TAB_ROUTES } from '../../routes';

const meta: Meta = { title: 'Layout', tags: ['autodocs'] };
export default meta;

const withRouter = (children: React.ReactNode) => (
  <MemoryRouter>{children}</MemoryRouter>
);

export const HeaderStory: StoryObj = { name: 'Header', render: () => withRouter(<Header />) };

export const SidebarNavStory: StoryObj = {
  name: 'SidebarNav',
  render: () => withRouter(<div style={{ height: 320 }}><SidebarNav tabs={TAB_ROUTES} /></div>),
};

export const BreadcrumbStory: StoryObj = {
  name: 'Breadcrumb',
  render: () => withRouter(
    <Breadcrumb items={[{ label: 'Home', to: '/' }, { label: 'Products', to: '/products' }, { label: 'Foo Product' }]} />,
  ),
};

export const PageWrapperStory: StoryObj = {
  name: 'PageWrapper',
  render: () => withRouter(
    <PageWrapper
      title="Products"
      breadcrumb={[{ label: 'Home', to: '/' }, { label: 'Products' }]}
    >
      <p style={{ padding: 20 }}>Page body content goes here.</p>
    </PageWrapper>,
  ),
};
