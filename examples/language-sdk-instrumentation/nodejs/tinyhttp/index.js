import { init, start } from '@pyroscope/nodejs';
import { App } from '@tinyhttp/app';
import { logger } from '@tinyhttp/logger';

const port = process.env['PORT'] || 5000;
const region = process.env['REGION'] || 'default';
const appName = process.env['APP_NAME'] || 'tinyhttp';
const pyroscopeUrl = process.env['PYROSCOPE_URL'] || 'http://pyroscope:4040';

init({
  appName: appName,
  serverAddress: pyroscopeUrl,
  tags: { region },
});
start();

const app = new App();
app.use(logger());

app.get('/', (_, res) => {
  res.send('Available routes are: /bike, /car, /scooter');
})

const genericSearchHandler = (p) => (_, res) => {
  const time = +new Date() + p * 1000;
  let i = 0;
  while (+new Date() < time) {
    i = i + Math.random();
  }
  res.send('Vehicle found');
};

app.get('/bike', (req, res) => {
  genericSearchHandler(0.5)(req, res);
});

app.get('/car', (req, res) => {
  genericSearchHandler(1)(req, res);
});

app.get('/scooter', (req, res) => {
  genericSearchHandler(0.25)(req, res);
});

app.listen(port, () => console.log(`Started on http://localhost:${port}`));
