console.log('puppeteer start');

const puppeteer = require('puppeteer');

function timeout(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

const args = process.argv.slice(2);

const from = args[1];
console.log(
  'from',
  from
)(async () => {
  const browser = await puppeteer.launch({});
  const page = await browser.newPage();
  await page.setViewport({ width: 1600, height: 1280 });
  await page.goto(
    `http://localhost:8080/d/65gjqY3Mk/main?orgId=1&from=${from}`,
    { waitUntil: 'networkidle2' }
  );
  await timeout(2000);
  await page.screenshot({
    captureBeyondViewport: false,
    path: args[0],
  });
  browser.close();
})();

console.log('puppeteer stop');
