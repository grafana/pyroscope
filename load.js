import http from 'k6/http';
import { check } from 'k6';

export let options = {
  vus: 5,
  duration: '10m',
};

export default function () {
  // 24h query
  const start = 1726473370933;
  const end = 1726617370933;

  const response = labelNamesRequest({
    matchers: ["{service_name=\"fire-dev-001/querier\"}"],
    start: start,
    end: end
  });

  const names = response.json('names');
  check(response, {
    'Label names response is 200': (r) => r.status === 200,
    'Label names response has data': (r) => names.length > 0,
  });

  for (let name of names) {
    const response = labelValuesRequest({
      matchers: [`{service_name=\"fire-dev-001/querier\", metric_name=\"${name}\"}`],
      start: start,
      end: end
    });

    const values = response.json('names');
    check(response, {
      'Label values response is 200': (r) => r.status === 200,
      'Label values response has data': () => values !== undefined && values.length > 0,
    });
  }
}

function labelNamesRequest(body) {
  return http.post('http://localhost:4100/querier.v1.QuerierService/LabelNames', JSON.stringify(body), {
    headers: {
      'Content-Type': 'application/json',
      'X-Scope-OrgID': '1218'
    },
  });
}

function labelValuesRequest(body) {
  return http.post('http://localhost:4100/querier.v1.QuerierService/LabelValues', JSON.stringify(body), {
    headers: {
      'Content-Type': 'application/json',
      'X-Scope-OrgID': '1218'
    },
  });
}
