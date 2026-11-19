#!/bin/sh
set -eu

cd "$(dirname "$0")"
version=$(node -e '
  const { packageManager } = require("./package.json");
  const [manager, version] = packageManager.split("@");
  if (manager !== "yarn" || !version) {
    throw new Error("packageManager must specify yarn@<version>");
  }
  process.stdout.write(version.split("+")[0]);
')

# Replace any preinstalled Yarn Classic binaries or Corepack shims.
npm install --global --force "@yarnpkg/cli-dist@$version"
