// this file is used to generate a message for each PR
import { markdown } from 'danger';
const fs = require('fs');
const path = require('path');

const report = fs.readFileSync(path.join(__dirname, './report/pr-report.md'));

markdown(`${report}`);
