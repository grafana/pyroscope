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
