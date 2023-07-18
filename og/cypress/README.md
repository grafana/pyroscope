# Cypress tests

[Cypress](https://www.cypress.io/) is a end-to-end testing library, used mainly to simulate real user scenario (ie. clicking around and interacting with).

# Running locally

While running the dev server, run `yarn cy:open`
Which will spawn a browser, click on the file that you want to work on. It will be refreshed automatically whenever you update that file.

Or run `yarn cy:ci` to run all tests.

# Writing tests

- Try to use [testids](https://kentcdodds.com/blog/making-your-ui-tests-resilient-to-change/) to select DOM elements
- Since our application is relatively simple, there's no need for Page Objects yet
- [Don't write small tests with single assertions](https://docs.cypress.io/guides/references/best-practices#Creating-tiny-tests-with-a-single-assertion)
- [Mock HTTP requests](https://docs.cypress.io/guides/guides/network-requests#Stub-Responses) when appropriate

# Visual testing
## tl;dr
To update the snapshots, run
```
yarn cy:ss
```
(`ss` stands for **screenshot**)

It requires docker installed and available under the `docker` binary in `PATH`.
(Not tested with `podman` or other alternatives)

To just run without updating the snapshots, run
```
yarn cy:ss-check
```

## Why
Part of our core functionality revolves around rendering a flamegraph **canvas**,
which is not straightforward to test.

We've decided to test it using [visual testing](https://docs.cypress.io/guides/tooling/visual-testing#Functional-vs-visual-testing).

## How

We use the [jaredpalmer/cypress-image-snapshot](https://github.com/jaredpalmer/cypress-image-snapshot) plugin to compare against "golden" screenshots.

By default, visual testing is disabled (we instead log a `Screenshot comparison is disabled` message). That happens because comparing images is flaky, since it depends on many factors like OS, browser, viewport and pixel density, to name a few.

Therefore we decided to update snapshots via a docker container, which is the same container that runs in ci. That way we have a consistent experience.

If we ever update the docker image (`cypress/included:8.4.1` at the time of this writing). We need to update in 2 places (`scripts/cypress-screenshots.sh` and in `.github/workflows/cypress-tests.yml`)


## Debugging visual tests
These can be very painful. We recommend recording videos (it's enabled by default), they help understand what's going on.

Feel free to add any tips as you learn about them.

## References
https://www.thisdot.co/blog/how-to-set-up-screenshot-comparison-testing-with-cypress-inside-an-nx
https://www.youtube.com/watch?v=1XQbGtRITys&list=PLP9o9QNnQuAYhotnIDEUQNXuvXL7ZmlyZ&index=14

# Updating cypress
There are 3 places to update:

- `package.json`
- `scripts/cypress-screenshots.sh`
- `.github/workflows/cypress-tests.yml`

They should be ALL in sync, otherwise you gonna have weird failures, specially with snapshot tests.

Don't forget to regenerate the snapshots (`yarn cy:ss`).
