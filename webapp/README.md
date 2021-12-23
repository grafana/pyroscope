# Webapp

# Tests
## Snapshot tests
Similar to what we do in cypress, we take snapshots of the canvas.

To make the results reproducible, we run in `docker`

To update the snapshots, run
```
yarn test:ss
```

To check the snapshots, run
```
yarn test:ss:check
```

Here ONLY tests matching the regex `group:snapshot` will run.
And the opposite is true, when running `yarn test`, these tests with `group:snapshot` in the name will be ignored.

# dependencies vs devDependencies
When installing a new package, consider the following:
Add to `dependencies` if it's **absolutely necessary to build** the application.
Anything else (local dev, CI) add too `devDependencies`.

The reasoning is that when building the docker image we install only `dependencies` required to build the application, by running `yarn install --production`.
Linting, testing etc is assumed to be ran in a different CI step.

# Using alias imports
Alias imports allow importing as if it was an external package, for example:
```javascript
import Button from '@ui/Button';
```

To be able to do that, you need to add the alias to the following files:
* `.storybook/main.js`
* `scripts/webpack/webpack.common.js`
* `tsconfig.json`
* `jest.config.js`
