import {markdown } from "danger"
const fs = require('fs');
const path = require('path');


const tableReport = fs.readFileSync(path.join(__dirname,"./table-report"));
const imgReport = fs.readFileSync(path.join(__dirname,"./image-report"));

// the markdown calls seem to be LIFO
// so it's easier to just use a single call
markdown(`${tableReport} \n${imgReport}`);

