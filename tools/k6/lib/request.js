import { check } from 'k6';
import http from 'k6/http';
import encoding from 'k6/encoding';

import { URL } from 'https://jslib.k6.io/url/1.0.0/index.js';

import { READ_TOKEN, TENANT_ID, BASE_URL } from './env.js';

export function doSelectMergeProfileRequest(body, headers) {
  const res = http.post(`${BASE_URL}/querier.v1.QuerierService/SelectMergeProfile`, JSON.stringify(body), {
    headers: withHeaders(headers),
    tags: { name: "querier.v1.QuerierService/SelectMergeProfile" }
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}

export function doRenderRequest(body, headers) {
  const params = new URL(`${BASE_URL}/pyroscope/render`);
  for (const [k, v] of Object.entries(body)) {
    params.searchParams.set(k, v);
  }

  const res = http.get(params.toString(), {
    headers: withHeaders(headers),
    tags: { name: '/render' },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}

export function doSelectMergeStacktracesRequest(body, headers) {
  const res = http.post(`${BASE_URL}/querier.v1.QuerierService/SelectMergeStacktraces`, JSON.stringify(body), {
    headers: withHeaders(headers),
    tags: { name: "querier.v1.QuerierService/SelectMergeStacktraces" }
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}

export function doLabelNamesRequest(body, headers) {
  const res = http.post(`${BASE_URL}/querier.v1.QuerierService/LabelNames`, JSON.stringify(body), {
    headers: withHeaders(headers),
    tags: { name: "querier.v1.QuerierService/LabelNames" }
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}

export function doSeriesRequest(body, headers) {
  const res = http.post(`${BASE_URL}/querier.v1.QuerierService/Series`, JSON.stringify(body), {
    headers: withHeaders(headers),
    tags: { name: "querier.v1.QuerierService/Series" }
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}

export function doRenderDiffRequest(body, headers) {
  const params = new URL(`${BASE_URL}/pyroscope/render-diff`);
  for (const [k, v] of Object.entries(body)) {
    params.searchParams.set(k, v);
  }

  const res = http.get(params.toString(), {
    headers: withHeaders(headers),
    tags: { name: '/render-diff' },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}

function withHeaders(headers) {
  const baseHeaders = {
    'User-Agent': 'k6-load-test',
    'Content-Type': 'application/json',
    'Authorization': `Basic ${encoding.b64encode(`${TENANT_ID}:${READ_TOKEN}`)}`
  };

  for (const [k, v] of Object.entries(headers || {})) {
    baseHeaders[k] = v;
  }
  return baseHeaders;
}
