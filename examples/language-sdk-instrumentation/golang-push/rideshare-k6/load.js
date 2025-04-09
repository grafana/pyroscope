import http from 'k6/http';
import { check, sleep } from 'k6';
import pyroscope from 'https://jslib.k6.io/http-instrumentation-pyroscope/1.0.1/index.js';
pyroscope.instrumentHTTP();

export let options = {
    scenarios: {
        low_load: {
            executor: 'constant-vus',
            vus: 5,
            duration: '30s',
        },
        high_load: {
            executor: 'ramping-vus',
            startVUs: 1,
            stages: [
                { duration: '10s', target: 10 }, // ramp up to 10 VUs
                { duration: '20s', target: 10 }, // stay at 10 VUs
                { duration: '10s', target: 0 },  // ramp down to 0 VUs
            ],
            gracefulRampDown: '5s',
        },
    },
};

export default function () {
    let res = http.get('http://XX.XX.XX.XX:5001/car'); // <-- change this to point to the domain & endpoint you want to test
    check(res, {
        'status is 200': (r) => r.status === 200,
    });
    sleep(1);
}