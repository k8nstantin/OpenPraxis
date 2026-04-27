import type { Meta, StoryObj } from '@storybook/react';
import { ErrorBoundary } from './ErrorBoundary';

function Boom(): JSX.Element {
  throw new Error('Something exploded in render');
}

const meta: Meta = { title: 'UI/ErrorBoundary', tags: ['autodocs'] };
export default meta;

export const CatchesError: StoryObj = {
  render: () => (
    <ErrorBoundary>
      <Boom />
    </ErrorBoundary>
  ),
};
