import type { StorybookConfig } from '@storybook/react-vite';

// Storybook is a dev-time tool only. It is NOT bundled into the Go
// binary — the production dashboard at /dashboard/ ships the Vite build
// and nothing more. `npm run storybook` is the operator's way to walk
// every primitive + chrome + cross-cutting component in isolation
// before signing off on a tab manifest cutover.
const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|tsx)'],
  addons: ['@storybook/addon-essentials'],
  framework: {
    name: '@storybook/react-vite',
    options: {},
  },
  core: { disableTelemetry: true },
};
export default config;
