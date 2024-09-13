import { check } from 'k6';
import http from 'k6/http';

export const options = {
  duration: '5m',
  vus: 3,
};

const URL = __ENV.TARGET_URL || 'http://localhost:5000';

export default function() {
  for (const endpoint of ['car', 'scooter', 'bike']) {
    const res = http.get(`${URL}/${endpoint}`);
    check(res, {
      'status is 200': (r) => r.status === 200,
    });
  }
}
