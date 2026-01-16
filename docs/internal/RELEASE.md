# Release Process

## Automatic Release Process

### GitHub Release

1. Create a new branch for the release (e.g., `release/vX.Y`).
   > [!IMPORTANT]
   > The release branch should only contain the major (X) and minor (Y) version, but not the patch level (Z), for example:
   >
   > ✅ Correct: `release/v1.3`
   >
   > ⚠️  Incorrect: `release/v1.3.0`  
2. Create the tag for the release (e.g., `vX.Y.Z`)
3. Push the release branch and tag to the remote. Note that the tag will kick off a release workflow via [goreleaser](https://github.com/grafana/pyroscope/actions/workflows/release.yml).
4. Create a GitHub label for backports:

   ```gh label create "backport release/vX.Y" -d "This label will backport a merged PR to the release/vX.Y branch" -c "#0052cc"```

The CI will automatically handle the build and then create and publish a new GitHub release.
Please do not delete GitHub releases that were once public.

#### GitHub Release Notes

Once the release is published, you should edit the release notes in GitHub. Use the already generated changelog and summarize it in the main categories:
- Features and enhancements
- (optional) Breaking changes
- Bug fixes
- Documentation updates

Keep the generated changelog as is, after the summary sections.

Make sure each release note has full links to the relevant pull requests.

### Website Release Notes

The list of changes from the GitHub release forms the basis of the public-facing release notes.
Release notes are added to the [public Pyroscope documentation](https://grafana.com/docs/pyroscope/latest/release-notes/).

#### GitHub Workflow

The website release notes need to be added to both the `release/vX.Y` branch and the main branch. The recommended workflow is to:
1. Add the changes with a PR against the main branch
2. Add a [backport](#backport) label and a `type/docs` label to the PR
3. Address feedback from reviewers and merge the PR

#### Writing Website Release Notes

The release notes follow the same pattern for each release:
1. Copy the previous release's page (i.e., V1-3.md) to the new release number. Change the version information and [page weight](https://grafana.com/docs/writers-toolkit/write/front-matter/#weight) in the file's frontmatter.
2. Update the page title (Version X.Y release notes) and add a few sentences about the main updates in the release.
3. Copy over the summary sections from the GitHub release notes. Make sure to use full links to the relevant pull requests.

For help writing release notes, refer to the [Writers' Toolkit](https://grafana.com/docs/writers-toolkit/write/).

### Helm Chart Update

Merge a PR that bumps the chart version in the main branch (no tagging required), the CI will automatically publish the chart to the [helm repository](https://grafana.github.io/helm-charts). 

## Backport

A PR to be backported must have the appropriate `backport release/vX.Y` label(s) AND one of [these expected labels](https://github.com/grafana/grafana-github-actions/blob/7d2b4af1112747f82e12adfbc00be44fecb3b616/backport/backport.ts#L16):
`['type/docs', 'type/bug', 'product-approved', 'type/ci']`. Note that these labels must be present before the PR is merged.

[Example backport PR](https://github.com/grafana/pyroscope/pull/4352)

## Patch Releases

When a patch release is needed, make PRs containing the necessary changes against the appropriate `release/vX.Y` branch.

Changes done in patch releases should be documented in the existing website release notes for that version under a new heading with
the version number. These documentation changes should be done with a PR against the appropriate release branch and then [backported](#backport) to the main branch.

Once the release notes are merged, a `vX.Y.Z` patch release tag must be created and pushed to remote to create a new release.

> [!WARNING]
> If you are releasing a patch version, for an older major/minor version (example:
> you are releasing `v1.15.2`, but the current latest release is `v1.16.0`),
> you need to make sure the release's actions to publish a `:latest` docker
> image tag and a `home-brew` formula are removed:
>
> This can be done by updating `release.yml` in the previous release branches to set `$IMAGE_PUBLISH_LATEST=false`.

## Manual Release Process

The release process uses [goreleaser](https://goreleaser.com/scm/github/?h=github#github) and can be configured
using the [.goreleaser.yaml](./.goreleaser.yaml).

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
