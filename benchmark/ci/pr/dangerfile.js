import {markdown } from "danger"
const fs = require('fs');
const path = require('path');


const bucketAddress = envOrFail("BUCKET_ADDRESS");
const prReport = fs.readFileSync(path.join(__dirname,"./pr-report"));

markdown(`${prReport}`);

const filenames = fs.readdirSync(path.join(__dirname,"./dashboard-screenshots"));
const img = (name, url) => `<img src="${url}" alt="drawing" width="400"/>`;
const imgmd = filenames.map(name => img(name, `${bucketAddress}/${name}`)).join(" ");

markdown(`
# screenshots
${imgmd}`);

function envOrFail(name) {
  const env = process.env[name];
  if (!env) {
    throw new Error(`ENV VAR ${name} is required.`)
  }
  return env;
}
