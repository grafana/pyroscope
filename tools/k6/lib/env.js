import { fail } from 'k6';

export const READ_TOKEN = __ENV.K6_READ_TOKEN || fail('K6_READ_TOKEN environment variable missing');
export const TENANT_ID = __ENV.K6_TENANT_ID || '1218';
export const BASE_URL = __ENV.K6_BASE_URL || 'http://profiles-dev-001.grafana-dev.net';
