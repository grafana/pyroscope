/* eslint-disable */
const Pyroscope = require('@pyroscope/nodejs');

const port = process.env['PORT'] || 5000;

const region = process.env['REGION'] || 'default';

const express = require('express');
const morgan = require('morgan');

const app = express();
app.use(morgan('dev'));
app.get('/', (req, res) => {
  res.send('Available routes are: /bike, /car, /scooter');
});

const genericSearchHandler = (p) => (req, res) => {
  const time = +new Date() + p * 1000;
  let i = 0;
  while (+new Date() < time) {
    i = i + Math.random();
  }
  res.send('Vehicle found');
};

Pyroscope.init({
  appName: 'nodejs',
  serverAddress: process.env['PYROSCOPE_SERVER'] || 'http://pyroscope:4040',
  tags: { region },
});

Pyroscope.start();

app.get('/bike', function bikeSearchHandler(req, res) {
  return genericSearchHandler(0.5)(req, res);
});
app.get('/car', function carSearchHandler(req, res) {
  return genericSearchHandler(1)(req, res);
});
app.get('/scooter', function scooterSearchHandler(req, res) {
  return genericSearchHandler(0.25)(req, res);
});

app.listen(port, () => {
  console.log(
    `Server has started on port ${port}, use http://localhost:${port}`
  );
});
