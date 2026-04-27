import type { Preview } from '@storybook/react';
import '../src/styles.css';
import '../src/components/ui/ui.css';

const preview: Preview = {
  parameters: {
    backgrounds: {
      default: 'dark',
      values: [{ name: 'dark', value: '#0f0f17' }],
    },
    a11y: {
      // axe-core options. Keep default ruleset; story-specific overrides
      // belong on the story.
      config: {},
    },
  },
};

export default preview;
