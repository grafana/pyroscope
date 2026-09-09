/* eslint-disable */
const Pyroscope = require('@pyroscope/nodejs');

const port = process.env['RIDESHARE_LISTEN_PORT'] || 5000;
const region = process.env['REGION'] || 'default';
const appName = process.env['PYROSCOPE_APPLICATION_NAME'] || 'express';
const pyroscopeUrl = process.env['PYROSCOPE_SERVER_ADDRESS'] || 'http://pyroscope:4040';
const pyroscopeUser = process.env['PYROSCOPE_BASIC_AUTH_USER'] || '';
const pyroscopePassword = process.env['PYROSCOPE_BASIC_AUTH_PASSWORD'] || '';

const express = require('express');
const morgan = require('morgan');

const app = express();
app.use(morgan('dev'));
app.get('/', (_, res) => {
  res.send('Available routes are: /bike, /car, /scooter');
});

const genericSearchHandler = (p) => (_, res) => {
  const time = +new Date() + p * 1000;
  let i = 0;
  while (+new Date() < time) {
    i = i + Math.random();
  }
  res.send('Vehicle found');
};

Pyroscope.init({
  appName: appName,
  serverAddress: pyroscopeUrl,
  basicAuthUser: pyroscopeUser,
  basicAuthPassword: pyroscopePassword,
  tags: { region },
});
Pyroscope.start();

app.get('/bike', function bikeSearchHandler(req, res) {
  Pyroscope.wrapWithLabels({ vehicle: 'bike' }, () =>
    genericSearchHandler(0.5)(req, res)
  );
});

app.get('/car', function carSearchHandler(req, res) {
  Pyroscope.wrapWithLabels({ vehicle: 'car' }, () =>
    genericSearchHandler(1)(req, res)
  );
});

app.get('/scooter', function scooterSearchHandler(req, res) {
  Pyroscope.wrapWithLabels({ vehicle: 'scooter' }, () =>
    genericSearchHandler(0.25)(req, res)
  );
});

app.listen(port, () => {
  console.log(
    `Server has started on port ${port}, use http://localhost:${port}`
  );
});
