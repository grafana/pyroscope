import {markdown } from "danger"
const fs = require('fs')


const filenames = fs.readdirSync("./dashboard-screenshots");
const bucketAddress = process.env.BUCKET_ADDRESS;

const img = (name, url) => `![${name}](${url})`

const md = filenames.map(name => img(name, `${bucketAddress}/${name}`)).join("\n"); 

markdown(`
# screenshots
${md}`);
