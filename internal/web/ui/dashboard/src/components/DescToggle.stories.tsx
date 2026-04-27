import type { Meta, StoryObj } from '@storybook/react';
import { DescToggle } from './DescToggle';

const RAW = `<task title="Foo">
  <role>Senior frontend</role>
  <scope>One sentence per acceptance bullet.</scope>
</task>`;
const RENDERED = '<p><strong>Senior frontend</strong> — one sentence per acceptance bullet.</p>';

const meta: Meta<typeof DescToggle> = { title: 'Components/DescToggle', component: DescToggle, tags: ['autodocs'] };
export default meta;

export const Markup: StoryObj<typeof DescToggle> = {
  args: { raw: RAW, rendered: RENDERED, modeOverride: 'markup' },
};
export const Rendered: StoryObj<typeof DescToggle> = {
  args: { raw: RAW, rendered: RENDERED, modeOverride: 'rendered' },
};
