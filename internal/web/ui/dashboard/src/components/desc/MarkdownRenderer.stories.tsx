import type { Meta, StoryObj } from '@storybook/react';
import { MarkdownRenderer } from './MarkdownRenderer';

const meta: Meta<typeof MarkdownRenderer> = {
  title: 'Cross-cutting / MarkdownRenderer',
  component: MarkdownRenderer,
};
export default meta;

export const FromMarkdown: StoryObj<typeof MarkdownRenderer> = {
  args: { source: '# Heading\n\nA paragraph with **bold** and `code`.', forceMode: 'rendered' },
};

export const FromHtml: StoryObj<typeof MarkdownRenderer> = {
  args: { html: '<p>Server-rendered <strong>HTML</strong> passes through verbatim.</p>', forceMode: 'rendered' },
};

export const Markup: StoryObj<typeof MarkdownRenderer> = {
  args: { source: '# Heading\n\n> The raw source.', forceMode: 'markup' },
};

export const Empty: StoryObj<typeof MarkdownRenderer> = { args: {} };
