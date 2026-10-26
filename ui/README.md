# Frontend development

Use Node.js 24 or newer. Install the pinned Yarn release without Corepack:

```sh
cd ui
sh ./install-yarn.sh
yarn install --immutable
yarn dev
```

The installer uses npm to install `@yarnpkg/cli-dist` globally, replacing any
existing Yarn binaries or Corepack shims. It requires a writable npm global prefix.
Docker and frontend CI use the same installer.

`packageManager` in `package.json` is the single source of truth for the Yarn
version. Renovate can track this pin using the existing repository configuration.
Builds always install the exact pinned version rather than the latest release.
