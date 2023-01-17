# Release Process

## Automatic Release Process

To release a new version of the project you need to follow the following steps:

1. Create a new branch for the release (e.g. `release/v0.1.0`)
2. Create tags for the release (e.g. `v0.1.0` and `phlare-0.1.0`)
3. Push the release branch and tags to the remote

The ci will automatically handle the build and create a draft github release.

Once ready, you can edit and publish the draft release on Github.

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
