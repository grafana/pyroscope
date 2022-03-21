# Lerna packages


# Local debugging
If you are trying to debug the publish of packages locally, use `verdaccio` as the registry:


```sh
npx verdaccio

verdaccio
npm set registry http://localhost:4873
npm adduser --registry http://localhost:4873
lerna publish
```

Then in your `lerna publish` command pass the `registry` flag pointing to `verdaccio`,
for example: `yarn lerna publish --registry=http://localhost:4873`

source: https://github.com/lerna/lerna/issues/51#issuecomment-348256663
