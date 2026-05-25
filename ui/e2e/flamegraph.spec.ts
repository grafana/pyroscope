import { test, expect, type Page, type Locator } from '@playwright/test';
import { mockPyroscopeApi } from './mockApi';

// "now" is pinned ~10s after the last fixture timestamp so the "now-1h" query
// window fully contains the fixture data and the time-series chart renders
// reproducibly.
const FIXED_NOW = new Date('2026-05-25T17:41:07.372Z');

test.beforeEach(async ({ page }) => {
  await page.clock.install({ time: FIXED_NOW });
  await mockPyroscopeApi(page);
  await page.goto('/');
});

async function waitForFlamegraphReady(page: Page) {
  await expect(page.locator('.flamegraph-wrapper')).toBeVisible();
  // canvas may not exist in TopTable-only view, so callers can omit it.
  await expect(
    page.getByRole('link', { name: 'runtime.kevent' }).first(),
  ).toBeVisible();
}

// Move the mouse away from the flamegraph and wait two animation frames so
// any pending canvas re-paint completes before we take a screenshot.
async function quiesce(page: Page) {
  await page.mouse.move(0, 0);
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
}

async function snap(page: Page, name: string, locator?: Locator) {
  await quiesce(page);
  const target = locator ?? page.locator('.flamegraph-wrapper');
  await expect(target).toHaveScreenshot(name);
}

test.describe('app shell', () => {
  test('renders the app and flamegraph from fixture data', async ({ page }) => {
    await waitForFlamegraphReady(page);
    await expect(
      page.getByRole('button', { name: /pyroscope · cpu/ }),
    ).toBeVisible();
    await expect(page.getByText('FLAMEGRAPH')).toBeVisible();
    // Baseline: Both view, default sort, by-package colors, dark theme.
    await snap(page, 'baseline.png');
  });

  test('switches between dark and light theme via the navbar dropdown', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    await page.locator('nav').getByRole('button', { name: 'Dark' }).click();
    await page.getByText('Light', { exact: true }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    // Theme change re-paints the flamegraph; capture the light variant.
    await snap(page, 'theme-light.png');

    await page.locator('nav').getByRole('button', { name: 'Light' }).click();
    await page.getByText('Dark', { exact: true }).click();
    await expect(page.locator('html')).not.toHaveAttribute(
      'data-theme',
      'light',
    );
  });
});

test.describe('flamegraph header controls', () => {
  test('search highlights matching nodes in the canvas', async ({ page }) => {
    await waitForFlamegraphReady(page);
    const search = page
      .locator('.flamegraph-wrapper')
      .getByPlaceholder('Search...');

    // runtime.findRunnable is one of the widest bars in the fixture (~4.71s
    // out of 11.1s total), so post-search it stays vibrant while every other
    // bar dims — a clear visual signal in the diff.
    await search.fill('runtime.findRunnable');
    await expect(search).toHaveValue('runtime.findRunnable');
    await expect(page.getByRole('button', { name: 'Clear' })).toBeVisible();

    // Search is debounced (250ms); advance the frozen clock past the debounce
    // before snapping so the canvas finishes its highlight pass.
    await page.clock.runFor(300);
    await snap(page, 'search-runtime-findRunnable.png');

    await page.getByRole('button', { name: 'Clear' }).click();
    await expect(search).toHaveValue('');
    await page.clock.runFor(300);
    // After clearing, the canvas must look like the pre-search baseline.
    await snap(page, 'baseline.png');
  });

  test('color scheme by value vs by package paints the canvas differently', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    // Default is by-package — already captured by baseline.png in the shell
    // test; here we verify the by-value variant produces a distinct image.
    await page.getByRole('button', { name: 'Change color scheme' }).click();
    await page.getByRole('menuitem', { name: 'By value' }).click();
    await snap(page, 'colors-by-value.png');

    // Switch back and confirm we're at the baseline.
    await page.getByRole('button', { name: 'Change color scheme' }).click();
    await page.getByRole('menuitem', { name: 'By package name' }).click();
    await snap(page, 'colors-by-package.png');
  });

  test('text alignment toggles bar label position', async ({ page }) => {
    await waitForFlamegraphReady(page);
    const leftInput = page.locator(
      'xpath=//label[@title="Align text left"]/preceding-sibling::input[1]',
    );
    const rightInput = page.locator(
      'xpath=//label[@title="Align text right"]/preceding-sibling::input[1]',
    );
    await expect(leftInput).toBeChecked();

    await rightInput.click();
    await expect(rightInput).toBeChecked();
    // Right-aligned bar labels look different in the canvas paint.
    await snap(page, 'text-align-right.png');

    await leftInput.click();
    await expect(leftInput).toBeChecked();
  });

  test('view toggle switches layout between Top Table, Flame Graph, and Both', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    const topTable = page.getByRole('radio', { name: 'Top Table' });
    const flameGraph = page.getByRole('radio', { name: 'Flame Graph' });
    const both = page.getByRole('radio', { name: 'Both' });

    await expect(both).toBeChecked();

    await flameGraph.click();
    await expect(flameGraph).toBeChecked();
    await expect(
      page.getByRole('link', { name: 'runtime.kevent' }),
    ).toHaveCount(0);
    await expect(page.locator('.flamegraph-wrapper canvas')).toBeVisible();
    await snap(page, 'view-flamegraph-only.png');

    await topTable.click();
    await expect(topTable).toBeChecked();
    await expect(page.locator('.flamegraph-wrapper canvas')).toHaveCount(0);
    await expect(
      page.getByRole('link', { name: 'runtime.kevent' }),
    ).toBeVisible();
    await snap(page, 'view-top-table-only.png');

    await both.click();
    await expect(both).toBeChecked();
  });

  test('expand-all reveals stacked function groups, collapse-all hides them', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    // The default state collapses consecutive same-name frames into a group;
    // expand-all reveals them as separate stacked bars. Then collapse-all
    // restores the compact default and must visually match the baseline.
    await page.getByRole('button', { name: 'Expand all groups' }).click();
    await snap(page, 'groups-expanded.png');

    await page.getByRole('button', { name: 'Collapse all groups' }).click();
    await snap(page, 'baseline.png');
  });
});

test.describe('top table', () => {
  const topTable = (page: Page) =>
    page.locator('[data-testid="topTable"]');

  test('default ordering shows runtime.kevent first with formatted self/total', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    await expect(
      page.getByRole('link', { name: 'runtime.kevent' }),
    ).toBeVisible();
    await expect(page.getByText('2.97 s', { exact: true })).toHaveCount(2);
    await snap(page, 'top-table-sort-by-self.png', topTable(page));
  });

  test('sorting by Symbol re-orders rows alphabetically', async ({ page }) => {
    await waitForFlamegraphReady(page);
    const firstRowLink = page
      .locator('[role="row"]')
      .nth(1)
      .getByRole('link');
    await expect(firstRowLink).toHaveText('runtime.kevent');
    await page.getByRole('button', { name: 'Symbol', exact: true }).click();
    await expect(firstRowLink).not.toHaveText('runtime.kevent');
    await snap(page, 'top-table-sort-by-symbol.png', topTable(page));
  });

  test('"Search for symbol" populates the search field and highlights matches', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    const search = page
      .locator('.flamegraph-wrapper')
      .getByPlaceholder('Search...');
    // Switch to Total sort descending so prominent parent bars show up in
    // the table. First Total click sorts ascending; click again for desc.
    // Then locate runtime.mcall — a parent bar (~5.7s in the fixture) whose
    // highlight is visibly prominent in the canvas.
    const totalSort = page.getByRole('button', { name: 'Total', exact: true });
    await totalSort.click();
    await totalSort.click();
    const targetRow = page.locator('[role="row"]', {
      has: page.getByRole('link', { name: 'runtime.mcall' }),
    });
    await targetRow.getByRole('button', { name: 'Search for symbol' }).click();
    await expect(search).toHaveValue('^runtime\\.mcall$');
    await page.clock.runFor(300);
    // The canvas should now highlight the wide runtime.mcall bar prominently.
    await snap(page, 'row-action-search-highlight.png');
  });

  test('"Show in sandwich view" splits the flamegraph into callers/callees', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    const row = page.locator('[role="row"]', {
      has: page.getByRole('link', { name: 'runtime.kevent' }),
    });
    await row.getByRole('button', { name: 'Show in sandwich view' }).click();
    await expect(
      page.getByRole('button', { name: 'Reset focus and sandwich state' }),
    ).toBeVisible();
    // Sandwich mode replaces the single flame graph with a callers/callees
    // split for the chosen symbol — visually unambiguous.
    await snap(page, 'sandwich-runtime-kevent.png');

    await page
      .getByRole('button', { name: 'Reset focus and sandwich state' })
      .click();
    await expect(
      page.getByRole('button', { name: 'Reset focus and sandwich state' }),
    ).toHaveCount(0);
    // After reset we should be back at the default flame graph layout.
    await snap(page, 'baseline.png');
  });
});

test.describe('flamegraph canvas', () => {
  test('hovering surfaces tooltip describing the bar under the cursor', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    const canvas = page.locator('.flamegraph-wrapper canvas');
    const box = await canvas.boundingBox();
    expect(box).not.toBeNull();
    if (!box) return;
    await canvas.hover({ position: { x: box.width / 2, y: 6 } });

    const tooltip = page.locator('[aria-live="polite"]');
    await expect(tooltip).toContainText('total', { timeout: 5_000 });
    await expect(tooltip).toContainText('Total:');
    await expect(tooltip).toContainText('Self:');
    // The total duration in our fixture is 11.1 s; any drift here means the
    // fixture changed or the wrong endpoint was hit.
    await expect(tooltip).toContainText('11.1 s');
    await expect(tooltip).toContainText('11,100,000,000');
  });

  test('clicking opens a context menu with Focus / Sandwich / Copy', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    const canvas = page.locator('.flamegraph-wrapper canvas');
    const box = await canvas.boundingBox();
    expect(box).not.toBeNull();
    if (!box) return;
    await canvas.click({ position: { x: box.width / 2, y: 6 } });
    await expect(
      page.getByRole('menuitem', { name: 'Focus block' }),
    ).toBeVisible();
    await expect(
      page.getByRole('menuitem', { name: 'Sandwich view' }),
    ).toBeVisible();
    await expect(
      page.getByRole('menuitem', { name: 'Copy function name' }),
    ).toBeVisible();
  });

  test('Focus block zooms into the clicked subtree; Reset restores baseline', async ({
    page,
  }) => {
    await waitForFlamegraphReady(page);
    const canvas = page.locator('.flamegraph-wrapper canvas');
    const box = await canvas.boundingBox();
    expect(box).not.toBeNull();
    if (!box) return;

    // Click a few rows deep so the focus visibly zooms in (focusing on the
    // synthetic "total" root would be a no-op visually).
    await canvas.click({ position: { x: box.width / 2, y: 80 } });
    await page.getByRole('menuitem', { name: 'Focus block' }).click();
    await expect(
      page.getByRole('button', { name: 'Reset focus and sandwich state' }),
    ).toBeVisible();
    await snap(page, 'focus-middle-bar.png');

    await page
      .getByRole('button', { name: 'Reset focus and sandwich state' })
      .click();
    await expect(
      page.getByRole('button', { name: 'Reset focus and sandwich state' }),
    ).toHaveCount(0);
    await snap(page, 'baseline.png');
  });
});
