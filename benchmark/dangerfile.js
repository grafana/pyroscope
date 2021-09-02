import {markdown } from "danger"
const fs = require('fs')

markdown(fs.readFileSync("report.md", "utf8"))
