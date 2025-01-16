# Release Process

## Automatic Release Process

To release a new version of the project you need to follow the following steps:

1. Create a new branch for the release (e.g., `release/vX.Y`)
2. Create the tag for the release (e.g., `vX.Y.Z`)
3. Push the release branch and tag to the remote
4. Create a GitHub label for backports:

   ```gh label create "backport release/vX.Y" -d "This label will backport a merged PR to the release/vX.Y branch" -c "#0052cc"```

> [!IMPORTANT]
> The release branch should only contain the major (X) and minor (Y) version, but not the patch level (Z), for example:
>
> ✅ Correct: `release/v1.3`
>
> ⚠️  Incorrect: `release/v1.3.0`

The CI will automatically handle the build and create a draft GitHub release.

Once ready, you can edit and publish the draft release on GitHub.

The list of changes from the GitHub release form the basis of the public-facing release notes. Release notes are added to the [public Pyroscope documentation](https://grafana.com/docs/pyroscope/latest/release-notes/). These release notes follow the same pattern for each release:

1. Copy the previous release's page (i.e., V1-3.md) to the new release number. Change the version information and [page weight](https://grafana.com/docs/writers-toolkit/write/front-matter/#weight) in the file's frontmatter.
2. Update the page title (Version x.x release notes) and add a few sentences about the main updates in the release.
3. Add a Features and enhancements section with list of updates. Refer to previous release notes for examples.
4. (optional) Add a Breaking changes section with a list of these changes and their impact (this section can also be used to update the [Upgrade page](https://grafana.com/docs/pyroscope/latest/upgrade-guide/)).
5. Add a Bug fixes section with a list of updates.
6. Add a Documentation updates section with a list of updates.

For help writing release notes, refer to the [Writers' Toolkit](https://grafana.com/docs/writers-toolkit/write/).

Please do not delete GitHub releases that were once public.

To release a patch version, simply merge fixes to the release branch then create and push a new tag. (e.g. `v0.x.1`). Any bugfixes or other entries should be added to the existing release notes for that version under a new heading with the version number.

> For helm charts, you need to merge a PR that bumps the chart version in the main branch (no tagging required), the CI will automatically publish the chart to the [helm repository](https://grafana.github.io/helm-charts).

## Manual Release Process

The release process uses [goreleaser](https://goreleaser.com/scm/github/?h=github#github) and can be configured
using the [.goreleaser.yml](./.goreleaser.yml).

To create a new release first prepare the release using:

```bash
make release/prepare
```

This will build and packages all artifacts without pushing or creating the GitHub release.

Once you're ready you can then tag your release.

```bash
git tag v0.1.0
```

And finally push the release using:

```bash
make release
```

> Make sure to have a [GitHub Token](https://goreleaser.com/scm/github/?h=github#github) `GITHUB_TOKEN` correctly set.

Make sure to create the release notes and CHANGELOG for any manual release.
