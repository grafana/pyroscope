/* eslint-disable */
import express from 'express';
import morgan from 'morgan';

const Pyroscope = require('@pyroscope/nodejs');
const SourceMapper = Pyroscope.default.SourceMapper;

const port = process.env['PORT'] || 5000;

const region = process.env['REGION'] || 'default';

const app = express();
app.use(morgan('dev'));

app.get('/', (req, res) => {
  res.send('Available routes are: /bike, /car, /scooter');
});

const genericSearchHandler = (p: number) => (req: any, res: any) => {
  const time = +new Date() + p * 1000;
  let i = 0;
  while (+new Date() < time) {
    i += Math.random();
  }
  res.send('Vehicle found');
};

app.get('/bike', function bikeSearchHandler(req, res) {
  Pyroscope.wrapWithLabels({ vehicle: 'bike' }, () =>
    genericSearchHandler(0.2)(req, res)
  );
});
app.get('/car', function carSearchHandler(req, res) {
  Pyroscope.wrapWithLabels({ vehicle: 'car' }, () =>
    genericSearchHandler(1)(req, res)
  );
});
app.get('/scooter', function scooterSearchHandler(req, res) {
  Pyroscope.wrapWithLabels({ vehicle: 'scooter' }, () =>
    genericSearchHandler(0.5)(req, res)
  );
});

SourceMapper.create(['.'])
  .then((sourceMapper) => {
    Pyroscope.init({
      appName: 'nodejs',
      serverAddress: 'http://pyroscope:4040',
      sourceMapper: sourceMapper,
      tags: { region },
    });
    Pyroscope.start();
  })
  .catch((e) => {
    console.error(e);
  });

app.listen(port, () => {
  console.log(
    `Server has started on port ${port}, use http://localhost:${port}`
  );
});
