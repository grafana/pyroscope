# Release Process

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
