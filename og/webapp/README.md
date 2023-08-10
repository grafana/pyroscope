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
import Button from '@phlare/ui/Button';
```

To be able to do that, you need to add the alias to the following files:
* `.storybook/main.js`
* `scripts/webpack/shared.ts`
* `tsconfig.json`
* `jest.config.js`

# Developing the webapp/templates page
By default, developing pages other than the index require a bit of setup:


For example, acessing http://locahlost:4040/forbidden won't work
To be able to access it, update the variable `pages` in `scripts/webpack.common.ts` to allow building all pages when in dev mode.

Beware, this will make the (local) build slower.

# Investigating webpack speed
Run with `--progress=profile` to get more info.

for example `yarn dev --progress=profile`


Another interesting flag is `--json`, which you can then analyze on https://chrisbateman.github.io/webpack-visualizer/

# Testing baseURL
It can be a bit of a pain in the ass.

Install nginx
```
nginx -c cypress/base-url/nginx.conf -g 'daemon off;'
```

Then run the server with `PYROSCOPE_BASE_URL=/pyroscope`

## Testing baseURL + auth
Same as before, but also run the `oauth2-mock-server`:
```
node scripts/oauth-mock/oauth-mock.js
```

Also run the server with
```
make dev SERVERPARAMS=--config=scripts/oauth-mock/pyroscope-config-base-url.yml
```
