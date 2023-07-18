#!/usr/bin/env node

const fs = require('fs');
const commandExists = require('command-exists').sync;
const { execSync } = require('child_process');

if (fs.existsSync('.git') && commandExists('git')) {
  // makes git blame ignore commits that are purely reformatting code
  execSync('git config blame.ignoreRevsFile .git-blame-ignore-revs');
}
