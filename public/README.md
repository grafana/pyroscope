# Frontend

## tl;dr

```bash
yarn install
# Make sure you have the backend running.
yarn dev
```

## Overrides

This repository currently uses [grafana/pyroscope] components, which then are overridden as necessary,
using typescript's alias and webpack alias configuration. See [tsconfig.json] and [webpack.common.js]
for more info.

[tsconfig.json]: ../tsconfig.json
[webpack.common.js]: ../scripts/webpack/webpack.common.js

### Guidelines for imports

It may be confusing to see different imports, so let's go over the most common examples:

`@pyroscope` -> Refers to code in this repository. Note that this is needed since other
downstream repositories may use this repository, and they also may want to override specific files.
