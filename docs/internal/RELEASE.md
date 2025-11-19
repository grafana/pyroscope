# Release Process

## Automatic Release Process

1. Update [release notes](#release-notes)
2. Create a new branch for the release (e.g., `release/vX.Y`). Make sure the release notes are present on this branch or
[backported](#backport) later.
3. Create the tag for the release (e.g., `vX.Y.Z`)
4. Push the release branch and tag to the remote. Note that the tag will kick off a real release via goreleaser.
5. Create a GitHub label for backports:

   ```gh label create "backport release/vX.Y" -d "This label will backport a merged PR to the release/vX.Y branch" -c "#0052cc"```

> [!IMPORTANT]
> The release branch should only contain the major (X) and minor (Y) version, but not the patch level (Z), for example:
>
> ✅ Correct: `release/v1.3`
>
> ⚠️  Incorrect: `release/v1.3.0`

The CI will automatically handle the build and create a draft GitHub release.

Once ready, you can edit and publish the draft release on GitHub.

Please do not delete GitHub releases that were once public.

### Release notes

The list of changes from the GitHub release form the basis of the public-facing release notes. Release notes are added to the [public Pyroscope documentation](https://grafana.com/docs/pyroscope/latest/release-notes/). These release notes follow the same pattern for each release:

1. Copy the previous release's page (i.e., V1-3.md) to the new release number. Change the version information and [page weight](https://grafana.com/docs/writers-toolkit/write/front-matter/#weight) in the file's frontmatter.
2. Update the page title (Version x.x release notes) and add a few sentences about the main updates in the release.
3. Add a Features and enhancements section with list of updates. Refer to previous release notes for examples.
4. (optional) Add a Breaking changes section with a list of these changes and their impact (this section can also be used to update the [Upgrade page](https://grafana.com/docs/pyroscope/latest/upgrade-guide/)).
5. Add a Bug fixes section with a list of updates.
6. Add a Documentation updates section with a list of updates.

For help writing release notes, refer to the [Writers' Toolkit](https://grafana.com/docs/writers-toolkit/write/).

### Helm charts update

Merge a PR that bumps the chart version in the main branch (no tagging required), the CI will automatically publish the chart to the [helm repository](https://grafana.github.io/helm-charts). 

## Backport

A PR to be backported must have the appropriate `backport release/vX.Y` label(s) AND one of [these expected labels](https://github.com/grafana/grafana-github-actions/blob/7d2b4af1112747f82e12adfbc00be44fecb3b616/backport/backport.ts#L16):
`['type/docs', 'type/bug', 'product-approved', 'type/ci']`. Note that these labels must be present before the PR is merged.

[Example backport PR](https://github.com/grafana/pyroscope/pull/4352)

## Patch releases

Any bugfixes or other entries should be added to the existing release notes for that version under a new heading with
the version number. The changes should be on the appropriate release branch.

Once merged, a `vX.Y.Z` patch release tag must be created and pushed to remote to create a new release.

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
