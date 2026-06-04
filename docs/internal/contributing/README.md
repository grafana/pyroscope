# Contributing

Welcome! We're excited that you're interested in contributing. Below are some basic guidelines.

## Workflow

Grafana Pyroscope follows a standard GitHub pull request workflow. If you're unfamiliar with this workflow, read the very helpful [Understanding the GitHub flow](https://guides.github.com/introduction/flow/) guide from GitHub.

You are welcome to create draft PRs at any stage of readiness - this
can be helpful to ask for assistance or to develop an idea. But before
a piece of work is finished it should:

- Be organised into one or more commits, each of which has a commit message that describes all changes made in that commit ('why' more than 'what' - we can read the diffs to see the code that changed).
- Each commit should build towards the whole - don't leave in back-tracks and mistakes that you later corrected.
- Have unit for new functionality or tests that would have caught the bug being fixed.
- If you have made any changes to flags, configs and/or protobuf definitions, run `make generate` and commit the changed files.

## Requirement

To be able to run make targets you'll need to install:

- [Go](https://go.dev/doc/install) (>= 1.25, see `go.mod`)
- [Docker](https://docs.docker.com/engine/install/)

All other required tools will be automatically downloaded `$(pwd)/.tmp/bin`.

> If you need a new CLI, we recommend you follow the same pattern and downloads requirement from the makefile.

## Formatting

Grafana Pyroscope uses [`golang-ci-lint`](https://github.com/golangci/golangci-lint) tool to format the Go files, and sort imports.
We use goimports with `-local github.com/grafana/pyroscope` parameter, to put Grafana Pyroscope internal imports into a separate group. We try to keep imports sorted into three groups: imports from standard library, imports of 3rd party packages and internal Grafana Pyroscope imports. Goimports will fix the order, but will keep existing newlines between imports in the groups. Avoid introducing newlines there.

Use `make lint` to ensure formatting is correct.

## Building Grafana Pyroscope

To build:

```
make go/bin
```

To run the unit test suite:

```
make go/test
```

To build the docker image use:

```
make docker-image/pyroscope/build
```

This target uses the `go/bin` target to first build binaries to include in the image.
Make sure to pass the correct `GOOS` and `GOARCH` env variables.

#### amd64 builds
```
make GOOS=linux GOARCH=amd64 docker-image/pyroscope/build
```

#### arm64 builds
```
make IMAGE_PLATFORM=linux/arm64 GOOS=linux GOARCH=arm64 docker-image/pyroscope/build
```

#### Running examples locally
replace `image: grafana/pyroscope` with the local tag name you got from docker-image/pyroscope/build (i.e):

```
  pyroscope:
    image: grafana/pyroscope:main-470125e1-WIP
    ports:
      - '4040:4040'
```

#### Run with Pyroscope with embedded Grafana + Profiles Drilldown

To quickly test the whole stack it is possible to run an embedded Grafana by using the target parameter:

```
go run ./cmd/pyroscope --target all,embedded-grafana
```

This will Pyroscope on `:4040` and the embedded Grafana on port `:4041`.

#### Frontend development

The frontend application is not in active development. While the UI it provides is usable and stable,
the recommended way to view and analyze profiling data is to use the
[Profiles Drilldown](https://grafana.com/docs/grafana/latest/visualizations/simplified-exploration/profiles/) Grafana app (pre-installed in recent Grafana versions).

If you do need to make changes to the frontend code, the following instructions should get you started.

The web UI lives in the `ui/` directory and is a dependency-minimal rewrite (React + Vite + TypeScript)
of the older `public/app` UI. See [`ui/CLAUDE.md`](../../../ui/CLAUDE.md) and
[`ui/DESIGN.md`](../../../ui/DESIGN.md) for the authoritative frontend guide.

The frontend uses **Yarn 4 (Berry)** and its `package.json` lives in `ui/` (not at the repository root).
To run it in development mode:

```sh
cd ui
yarn install
yarn dev
```

This serves the app at `http://localhost:5173` and proxies API requests to a Pyroscope server at
`http://localhost:4040`. In a separate terminal, start a server for it to talk to, for example:

```sh
go run ./cmd/pyroscope
```

### Dependency management

We use [Go modules](https://golang.org/cmd/go/#hdr-Modules__module_versions__and_more) to manage dependencies on external packages.
However, we don't commit the `vendor/` folder.

To add or update a new dependency, use the `go get` command:

```bash
# Pick the latest tagged release.
go get example.com/some/module/pkg

# Pick a specific version.
go get example.com/some/module/pkg@vX.Y.Z
```

Tidy up the `go.mod` and `go.sum` files:

```bash
make go/mod
```

Commit the changes to `go.mod` and `go.sum` before submitting your pull request.

## Documentation

The Grafana Pyroscope documentation is compiled into a website published at [grafana.com](https://grafana.com/).

To start the website locally you can use `make docs/docs`. The command will print instructions on how to access the website.

Note: if you attempt to view pages on GitHub, it's likely that you might find broken links or pages. That is expected and should not be addressed unless it is causing issues with the site that occur as part of the build.
