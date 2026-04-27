// Single axe-core suite that walks every primitive + chrome + cross-cutting
// component story and asserts no a11y violations. Storybook's own runner
// (`@storybook/test-runner`) would run this against the live Storybook
// server; we intentionally keep it as plain vitest so the a11y gate runs
// in `make test` without spinning up the Storybook process.
import { render, cleanup } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import axe from 'axe-core';
import { MemoryRouter } from 'react-router-dom';
import type { ReactElement } from 'react';

import { Button } from '@/components/ui/Button';
import { IconButton } from '@/components/ui/IconButton';
import { Badge } from '@/components/ui/Badge';
import { StatusDot } from '@/components/ui/StatusDot';
import { EmptyState } from '@/components/ui/EmptyState';
import { Header } from '@/components/layout/Header';
import { Breadcrumb } from '@/components/layout/Breadcrumb';
import { PageWrapper } from '@/components/layout/PageWrapper';
import { DescToggle } from '@/components/desc/DescToggle';
import { MarkdownRenderer } from '@/components/desc/MarkdownRenderer';
import { CommentsList } from '@/components/comments/CommentsList';
import { CommentEditor } from '@/components/comments/CommentEditor';
import { FormField } from '@/components/forms/FormField';

afterEach(() => cleanup());

async function expectNoViolations(node: ReactElement) {
  const { container } = render(node);
  const results = await axe.run(container, {
    // jsdom can't compute color-contrast — disable that single rule rather
    // than the whole color suite so the rest still runs.
    rules: { 'color-contrast': { enabled: false } },
  });
  if (results.violations.length > 0) {
    const summary = results.violations.map((v) => `${v.id}: ${v.help}`).join('\n');
    throw new Error(`axe violations:\n${summary}`);
  }
  expect(results.violations).toHaveLength(0);
}

describe('a11y — primitives', () => {
  it('Button is accessible', () => expectNoViolations(<Button>Save</Button>));
  it('IconButton has accessible name', () => expectNoViolations(<IconButton icon="×" label="Close" />));
  it('Badge is accessible', () => expectNoViolations(<Badge>open</Badge>));
  it('StatusDot has accessible name', () => expectNoViolations(<StatusDot status="open" label="open" />));
  it('EmptyState is accessible', () => expectNoViolations(<EmptyState message="No data" />));
});

describe('a11y — chrome', () => {
  it('Header is accessible', () =>
    expectNoViolations(
      <MemoryRouter><Header /></MemoryRouter>,
    ));
  it('Breadcrumb is accessible', () =>
    expectNoViolations(
      <MemoryRouter><Breadcrumb items={[{ label: 'Home', to: '/' }, { label: 'Now' }]} /></MemoryRouter>,
    ));
  it('PageWrapper is accessible', () =>
    expectNoViolations(
      <MemoryRouter>
        <PageWrapper title="Products" breadcrumbs={[{ label: 'Home', to: '/' }, { label: 'Products' }]}>
          <p>body</p>
        </PageWrapper>
      </MemoryRouter>,
    ));
});

describe('a11y — cross-cutting', () => {
  it('DescToggle is accessible', () => expectNoViolations(<DescToggle />));
  it('MarkdownRenderer is accessible', () =>
    expectNoViolations(<MarkdownRenderer source="**hello**" forceMode="rendered" />));
  it('CommentsList is accessible', () =>
    expectNoViolations(
      <CommentsList
        comments={[{ id: '1', author: 'a', body: 'b', created_at: new Date().toISOString() }]}
      />,
    ));
  it('CommentEditor is accessible', () =>
    expectNoViolations(<CommentEditor onSubmit={() => {}} />));
  it('FormField is accessible', () =>
    expectNoViolations(
      <FormField label="Title" htmlFor="title">
        <input id="title" type="text" />
      </FormField>,
    ));
});
