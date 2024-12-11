import http from 'k6/http';
import { check } from 'k6';
import pyroscope from 'https://jslib.k6.io/http-instrumentation-pyroscope/1.0.1/index.js';

pyroscope.instrumentHTTP();

export let options = {
  vus: 3,
  duration: '1m',
};

export default function () {
  const vehicles = [
    'car',
    'scooter',
    'bike',
  ];

  for (const v of vehicles) {
    const req = http.get(`http://localhost:5001/${v}`);
    check(req, {
      'status is 200': (r) => r.status === 200,
    });
  }
}
