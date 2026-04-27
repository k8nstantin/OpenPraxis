// Single axe-core suite that walks every primitive + chrome + cross-cutting
// component story and asserts no a11y violations. Storybook's own runner
// (`@storybook/test-runner`) would run this against the live Storybook
// server; we intentionally keep it as plain vitest so the a11y gate runs
// in `make test` without spinning up the Storybook process.
import { render, cleanup, act } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import axe from 'axe-core';
import { MemoryRouter } from 'react-router-dom';
import { useEffect, type ReactElement } from 'react';

import { Button } from '@/components/ui/Button';
import { IconButton } from '@/components/ui/IconButton';
import { Badge } from '@/components/ui/Badge';
import { StatusDot } from '@/components/ui/StatusDot';
import { EmptyState } from '@/components/ui/EmptyState';
import { Dialog } from '@/components/ui/Dialog';
import { Tooltip, TooltipProvider } from '@/components/ui/Tooltip';
import { Popover } from '@/components/ui/Popover';
import { ErrorBoundary } from '@/components/ui/ErrorBoundary';
import { Toaster, toast } from '@/components/ui/Toast';
import { Header } from '@/components/layout/Header';
import { Breadcrumb } from '@/components/layout/Breadcrumb';
import { PageWrapper } from '@/components/layout/PageWrapper';
import { DescToggle } from '@/components/desc/DescToggle';
import { MarkdownRenderer } from '@/components/desc/MarkdownRenderer';
import { CommentsList } from '@/components/comments/CommentsList';
import { CommentEditor } from '@/components/comments/CommentEditor';
import { FormField } from '@/components/forms/FormField';

afterEach(() => cleanup());

const AXE_OPTS = {
  // jsdom can't compute color-contrast — disable that single rule rather
  // than the whole color suite so the rest still runs.
  rules: { 'color-contrast': { enabled: false } },
};

async function expectNoViolations(node: ReactElement) {
  const { container } = render(node);
  const results = await axe.run(container, AXE_OPTS);
  if (results.violations.length > 0) {
    const summary = results.violations.map((v) => `${v.id}: ${v.help}`).join('\n');
    throw new Error(`axe violations:\n${summary}`);
  }
  expect(results.violations).toHaveLength(0);
}

// Radix-portal primitives (Dialog, Tooltip, Popover) render their content
// into a portal mounted at document.body, not inside the test's render
// container. Scan document.body so axe sees the portal.
async function expectNoViolationsBody(node: ReactElement) {
  render(node);
  const results = await axe.run(document.body, AXE_OPTS);
  if (results.violations.length > 0) {
    const summary = results.violations.map((v) => `${v.id}: ${v.help}`).join('\n');
    throw new Error(`axe violations:\n${summary}`);
  }
  expect(results.violations).toHaveLength(0);
}

// Helper that throws on first render — exercises the ErrorBoundary
// fallback path so axe scans the role="alert" surface, not happy-path
// children. `: never` is honest (function never returns); also
// satisfies TS 5+'s rule that JSX components must return ReactNode,
// not void.
function ThrowOnce({ msg = 'boom' }: { msg?: string }): never {
  throw new Error(msg);
}

describe('a11y — primitives', () => {
  it('Button is accessible', () => expectNoViolations(<Button>Save</Button>));
  it('IconButton has accessible name', () => expectNoViolations(<IconButton icon="×" label="Close" />));
  it('Badge is accessible', () => expectNoViolations(<Badge>open</Badge>));
  it('StatusDot has accessible name', () => expectNoViolations(<StatusDot status="open" label="open" />));
  it('EmptyState is accessible', () => expectNoViolations(<EmptyState message="No data" />));

  // Cycle 1 + 2 review (T2) flagged these 5 as missing axe coverage.
  // The Radix-portal primitives render into document.body; scan body
  // (not just the test container) so axe sees the portalled content.
  it('Dialog is accessible (open state)', () =>
    expectNoViolationsBody(
      <Dialog open onOpenChange={() => {}} title="Confirm" description="Are you sure?" footer={<Button>OK</Button>}>
        <p>Body content</p>
      </Dialog>,
    ));

  it('Tooltip is accessible (provider + zero-delay open)', () =>
    expectNoViolationsBody(
      <TooltipProvider>
        <Tooltip content="hint text" delayDuration={0}>
          <button type="button" aria-label="action">action</button>
        </Tooltip>
      </TooltipProvider>,
    ));

  it('Popover is accessible (open state)', () =>
    expectNoViolationsBody(
      <Popover
        open
        onOpenChange={() => {}}
        trigger={<button type="button">trigger</button>}
        ariaLabel="Filter options"
      >
        <p>Popover content</p>
      </Popover>,
    ));

  it('ErrorBoundary fallback is accessible (role=alert)', () =>
    expectNoViolations(
      <ErrorBoundary>
        <ThrowOnce />
      </ErrorBoundary>,
    ));

  it('Toaster + dispatched toast is accessible', async () => {
    function ToastEmitter() {
      useEffect(() => {
        toast.success('Saved successfully');
      }, []);
      return null;
    }
    await act(async () => {
      render(
        <>
          <Toaster />
          <ToastEmitter />
        </>,
      );
    });
    const results = await axe.run(document.body, AXE_OPTS);
    if (results.violations.length > 0) {
      const summary = results.violations.map((v) => `${v.id}: ${v.help}`).join('\n');
      throw new Error(`axe violations:\n${summary}`);
    }
    expect(results.violations).toHaveLength(0);
  });
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
