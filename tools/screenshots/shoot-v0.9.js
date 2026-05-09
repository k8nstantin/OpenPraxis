const { chromium } = require('playwright');
const path = require('path');

const BASE = 'http://localhost:9966';
const OUT  = '/Users/calexander/gryphon/openpraxis/docs/screenshots/v0.9';

async function nav(pg, href) {
  await pg.goto(`${BASE}${href}`, { waitUntil: 'networkidle' });
  await pg.waitForTimeout(2000);
}

async function clickFirst(pg, selector) {
  const el = pg.locator(selector).first();
  if (await el.count() > 0) { await el.click(); await pg.waitForTimeout(1500); }
}

async function clickTab(pg, name) {
  const t = pg.locator(`[role=tab]:has-text("${name}")`).first();
  if (await t.count() > 0) { await t.click(); await pg.waitForTimeout(1200); }
}

async function shot(pg, file) {
  await pg.screenshot({ path: path.join(OUT, file), fullPage: false });
  console.log('✓', file);
}

const ROW = 'tr[data-entity-uid], tr[class*="cursor-pointer"], button[class*="flex min-w-0"]';

(async () => {
  const browser = await chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark' });
  const pg  = await ctx.newPage();

  // Overview — highlight total turns line
  await nav(pg, '/');
  await shot(pg, 'overview.png');

  // Products list
  await nav(pg, '/products');
  await shot(pg, 'products.png');

  // Product detail — new trace-grounded product with full DAG
  await nav(pg, '/products');
  await clickFirst(pg, 'a[href*="product"], tr[class*="cursor"]');
  await shot(pg, 'product-detail.png');

  // Product DAG — shows skill → product → manifests → tasks + idea
  await clickTab(pg, 'DAG');
  await pg.waitForTimeout(2500);
  await shot(pg, 'product-dag.png');

  // Manifests list
  await nav(pg, '/manifests');
  await shot(pg, 'manifests.png');

  // Tasks list
  await nav(pg, '/tasks');
  await shot(pg, 'tasks.png');

  // Task detail — shows prior_context in prompt
  await clickFirst(pg, 'tr[class*="cursor"], a[href*="task"]');
  await shot(pg, 'task-detail.png');

  // Task Settings tab — shows new prompt context knobs
  await clickTab(pg, 'Settings');
  await shot(pg, 'task-settings-prompt-context.png');

  // Settings page — Catalog view with new groups
  await nav(pg, '/settings');
  await shot(pg, 'settings-catalog.png');

  // Settings — Scope Editor
  await pg.locator('a[href*="scope"], button:has-text("Scope")').first().click().catch(() => {});
  await pg.waitForTimeout(1200);
  await shot(pg, 'settings-scope-editor.png');

  // Stats / Activity
  await nav(pg, '/stats');
  await shot(pg, 'stats-overview.png');

  // Ideas
  await nav(pg, '/ideas');
  await shot(pg, 'ideas.png');

  // Skills
  await nav(pg, '/skills');
  await shot(pg, 'skills.png');

  await browser.close();
  console.log('\nAll screenshots saved to', OUT);
})().catch(e => { console.error(e); process.exit(1); });
