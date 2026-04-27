import type { Meta, StoryObj } from '@storybook/react';
import { MarkdownRenderer } from './MarkdownRenderer';

const meta: Meta<typeof MarkdownRenderer> = {
  title: 'Components/MarkdownRenderer',
  component: MarkdownRenderer,
  tags: ['autodocs'],
};
export default meta;

export const TrustedHtml: StoryObj<typeof MarkdownRenderer> = {
  args: {
    source: '<h3>Hello</h3><p>Trusted passthrough renders raw HTML verbatim.</p>',
    trustHtml: true,
  },
};
export const Markdown: StoryObj<typeof MarkdownRenderer> = {
  args: {
    source: '# Heading\n\nParagraph with **bold** and `code`.',
    trustHtml: false,
  },
};
