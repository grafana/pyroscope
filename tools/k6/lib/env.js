import { fail } from 'k6';

export const READ_TOKEN = __ENV.K6_READ_TOKEN;
export const TENANT_ID = __ENV.K6_TENANT_ID;
export const BASE_URL = __ENV.K6_BASE_URL || fail('K6_BASE_URL environment variable missing');
