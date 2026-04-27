import type { Meta, StoryObj } from '@storybook/react';
import { ErrorBoundary } from './ErrorBoundary';

const Bomb = () => {
  throw new Error('Bomb went off');
};

const meta: Meta<typeof ErrorBoundary> = {
  title: 'UI / ErrorBoundary',
  component: ErrorBoundary,
};
export default meta;

export const Caught: StoryObj<typeof ErrorBoundary> = {
  render: () => (
    <ErrorBoundary>
      <Bomb />
    </ErrorBoundary>
  ),
};
