console.log('puppeteer start');

const puppeteer = require('puppeteer');

function timeout(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
};

const args = process.argv.slice(2);

const from = args[1];

(async() => {
  const browser = await puppeteer.launch({
    // headless: false,
    args: [
      '--disable-background-networking',
      '--disable-background-timer-throttling',
      '--disable-client-side-phishing-detection',
      '--disable-default-apps',
      '--disable-extensions',
      '--disable-hang-monitor',
      '--disable-popup-blocking',
      '--disable-prompt-on-repost',
      '--disable-sync',
      '--disable-translate',
      '--metrics-recording-only',
      '--no-first-run',
      '--remote-debugging-port=0',
      '--safebrowsing-disable-auto-update',
      '--enable-automation',
      '--password-store=basic',
      '--use-mock-keychain',
      // '--user-data-dir=/tmp/puppeteer_dev_profile-GhEAXZ',
      '--headless',
      '--disable-gpu',
      '--hide-scrollbars',
      '--mute-audio',
      '--no-sandbox',
      '--disable-setuid-sandbox'
    ]
  });
  const page = await browser.newPage();
  await page.setViewport({width: 1600, height: 1280})
  await page.goto('http://localhost:8080/d/65gjqY3Mk/main?orgId=1&from='+from, {waitUntil: 'networkidle2'});
  await timeout(2000);
  await page.screenshot({
    captureBeyondViewport: false,
    path: args[0]
  });
  browser.close();
})();

console.log('puppeteer stop');
