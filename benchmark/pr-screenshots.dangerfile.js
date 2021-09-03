import {markdown } from "danger"
const fs = require('fs')


const filenames = fs.readdirSync("./dashboard-screenshots");
markdown(`
# report
generated files ${filenames}
`);
