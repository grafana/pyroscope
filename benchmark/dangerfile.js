// this file is used to generate a message for each PR
import { markdown } from 'danger';
const fs = require('fs');
const path = require('path');

const metaReport = fs.readFileSync(path.join(__dirname, './meta-report'));
const tableReport = fs.readFileSync(path.join(__dirname, './table-report'));
const imgReport = fs.readFileSync(path.join(__dirname, './image-report'));

// the markdown calls seem to be LIFO
// so it's easier to just use a single call
markdown(`${metaReport} \n${tableReport} \n${imgReport}`);
