import type { Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));

function readFixture(name: string): string {
  return readFileSync(join(here, 'fixtures', name), 'utf8');
}

const labelNames = readFixture('labelnames.json');
const series = readFixture('series.json');
const flamegraph = readFixture('flamegraph.json');
const selectSeries = readFixture('selectseries.json');

const json = (body: string) => ({
  status: 200,
  contentType: 'application/json',
  body,
});

// Routes Pyroscope QuerierService calls to committed fixtures, so the e2e
// suite renders an identical flamegraph every run regardless of the backend.
export async function mockPyroscopeApi(page: Page): Promise<void> {
  await page.route('**/querier.v1.QuerierService/LabelNames', (route) =>
    route.fulfill(json(labelNames)),
  );
  await page.route('**/querier.v1.QuerierService/Series', (route) =>
    route.fulfill(json(series)),
  );
  await page.route(
    '**/querier.v1.QuerierService/SelectMergeStacktraces',
    (route) => route.fulfill(json(flamegraph)),
  );
  await page.route('**/querier.v1.QuerierService/SelectSeries', (route) =>
    route.fulfill(json(selectSeries)),
  );
}
