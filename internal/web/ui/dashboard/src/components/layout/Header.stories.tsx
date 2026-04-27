import type { Meta, StoryObj } from '@storybook/react';
import { MemoryRouter } from 'react-router-dom';
import { Header } from './Header';

const meta: Meta<typeof Header> = {
  title: 'Chrome / Header',
  component: Header,
  decorators: [(Story) => <MemoryRouter><Story /></MemoryRouter>],
};
export default meta;

export const Default: StoryObj<typeof Header> = {};
