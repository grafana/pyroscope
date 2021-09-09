import {markdown } from "danger"
const fs = require('fs');
const path = require('path');


const tableReport = fs.readFileSync(path.join(__dirname,"./table-report"));
const imgReport = fs.readFileSync(path.join(__dirname,"./image-report"));

markdown(`${tableReport}`);
markdown(`${imgReport}`);

