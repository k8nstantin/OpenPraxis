const { chromium } = require('playwright');
const path = require('path');
const OUT = '/Users/calexander/openloom-serve/docs/images';
const ROW = 'button.flex.min-w-0, button[class*="flex min-w-0"]';

async function nav(pg, text) {
  await pg.locator(`text="${text}"`).first().click();
  await pg.waitForTimeout(2500);
}

async function clickRow(pg) {
  const r = pg.locator(ROW).first();
  if (await r.count() > 0) { await r.click(); await pg.waitForTimeout(1800); }
}

async function clickTab(pg, name) {
  const t = pg.locator(`[role=tab]:has-text("${name}"), button[role=tab]:has-text("${name}")`).first();
  if (await t.count() > 0) { await t.click(); await pg.waitForTimeout(1200); }
}

async function shot(pg, file) {
  await pg.screenshot({ path: path.join(OUT, file) });
  console.log('saved', file);
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark' });
  const pg  = await ctx.newPage();
  await pg.goto('http://localhost:8765', { waitUntil: 'networkidle' });
  await pg.waitForTimeout(4000);

  // Overview
  await shot(pg, 'overview.png');

  // Tasks — with row selected to show detail panel
  await nav(pg, 'Tasks');
  await clickRow(pg);
  await shot(pg, 'tasks-live-output.png');

  // Products — list view
  await nav(pg, 'Products');
  await shot(pg, 'products-detail.png');

  // Products — click row then DAG tab
  await clickRow(pg);
  const dagTab = pg.locator('[role=tab]:has-text("DAG")').first();
  if (await dagTab.count() > 0) { await dagTab.click(); await pg.waitForTimeout(2500); }
  await shot(pg, 'product-dag-openpraxis.png');

  // Manifests — with row selected
  await nav(pg, 'Manifests');
  await clickRow(pg);
  await shot(pg, 'manifests-detail.png');

  // Audit / Watcher
  await nav(pg, 'Audit');
  await shot(pg, 'watcher-audit-history.png');

  // Activity → conversations
  await nav(pg, 'Activity');
  await shot(pg, 'conversations-detail.png');

  // Exec controls: Products > row > Settings tab (if exists, else just row view)
  await nav(pg, 'Products');
  await clickRow(pg);
  await clickTab(pg, 'Settings');
  await shot(pg, 'exec-controls-product.png');

  await nav(pg, 'Manifests');
  await clickRow(pg);
  await clickTab(pg, 'Settings');
  await shot(pg, 'exec-controls-manifest.png');

  await nav(pg, 'Tasks');
  await clickRow(pg);
  await clickTab(pg, 'Settings');
  await shot(pg, 'exec-controls-task.png');

  await browser.close();
  console.log('\nAll done.');
})().catch(e => { console.error(e.message); process.exit(1); });
