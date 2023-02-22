# Release Process

## Automatic Release Process

To release a new version of the project you need to follow the following steps:

1. Create a new branch for the release (e.g. `release/v0.x`)
2. Create the tag for the release (e.g. `v0.x.0`)
3. Push the release branch and tag to the remote

The ci will automatically handle the build and create a draft github release.

Once ready, you can edit and publish the draft release on Github.

To release a minor version simply merge fixes to the release branch then create and push a new tag. (e.g. `v0.x.1`)

> For helm charts, you need to merge a PR that bumps the chart version in the main branch (no tagging required), the ci will automatically publish the chart to the [helm repository](https://grafana.github.io/helm-charts).
>
## Manual Release Process

The release process uses [goreleaser](https://goreleaser.com/scm/github/?h=github#github) and can be configured
using the [.goreleaser.yml](./.goreleaser.yml).

To create a new release first prepare the release using:

```bash
make release/prepare
```

This will build and packages all artifacts without pushing or creating the github release.

Once you're ready you can then tag your release.

```bash
git tag v0.1.0
```

And finally push the release using:

```bash
make release
```

> Make sure to have a [Github Token](https://goreleaser.com/scm/github/?h=github#github) `GITHUB_TOKEN` correctly set.
