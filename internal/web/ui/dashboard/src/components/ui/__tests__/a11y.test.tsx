import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import axe from 'axe-core';
import { Button } from '../Button';
import { IconButton } from '../IconButton';
import { Badge } from '../Badge';
import { StatusDot } from '../StatusDot';
import { EmptyState } from '../EmptyState';

// axe-core scans the rendered DOM for accessibility violations. Each
// primitive gets at least one render pass so the moment a regression
// strips an aria-label or breaks contrast the suite turns red.

async function runAxe(node: Element): Promise<axe.AxeResults> {
  return axe.run(node, {
    runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa'] },
    // jsdom can't lay out colors / fonts, so color-contrast scans are
    // unreliable in vitest. Storybook's a11y addon catches contrast in
    // the real browser.
    rules: { 'color-contrast': { enabled: false } },
  });
}

describe('a11y — UI primitives', () => {
  it('Button — primary has no violations', async () => {
    const { container } = render(<Button variant="primary">Save</Button>);
    const r = await runAxe(container);
    expect(r.violations).toEqual([]);
  });

  it('IconButton — required aria-label scans clean', async () => {
    const { container } = render(<IconButton aria-label="Close" icon={<span aria-hidden>×</span>} />);
    const r = await runAxe(container);
    expect(r.violations).toEqual([]);
  });

  it('Badge — neutral tone scans clean', async () => {
    const { container } = render(<Badge tone="info">42</Badge>);
    const r = await runAxe(container);
    expect(r.violations).toEqual([]);
  });

  it('StatusDot — has aria-label fallback', async () => {
    const { container } = render(<StatusDot status="running" label="Task running" />);
    const r = await runAxe(container);
    expect(r.violations).toEqual([]);
  });

  it('EmptyState — error tone uses role=alert', async () => {
    const { container } = render(<EmptyState tone="error" title="Failed" description="HTTP 500" />);
    const r = await runAxe(container);
    expect(r.violations).toEqual([]);
  });
});
