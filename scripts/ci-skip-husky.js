const { execSync } = require('child_process');

// https://github.com/yarnpkg/yarn/issues/7212#issuecomment-1136443313
if (!process.env.CI) {
  console.log('Executing husky install...');
  execSync('husky install');
}
