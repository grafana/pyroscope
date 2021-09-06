import {markdown } from "danger"
const fs = require('fs');
const path = require('path');


const bucketAddress = envOrFail("BUCKET_ADDRESS");
const filenames = fs.readdirSync(path.join(__dirname,"./dashboard-screenshots"));

const img = (name, url) => `![${name}](${url})`

const md = filenames.map(name => img(name, `${bucketAddress}/${name}`)).join("\n"); 

markdown(`
# screenshots
${md}`);

function envOrFail(name) {
  const env = process.env[name];
  if (!env) {
    throw new Error(`ENV VAR ${name} is required.`)
  }
  return env;
}
