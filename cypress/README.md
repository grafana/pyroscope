# Cypress tests

[Cypress](https://www.cypress.io/) is a end-to-end testing library, used mainly to simulate real user scenario (ie. clicking around and interacting with).


# Running locally
While running the dev server, run `yarn cy:open`
Which will spawn a browser, click on the file that you want to work on. It will be refreshed automatically whenever you update that file.

Or run `yarn cy:ci` to run all tests.

# Writing tests
* Try to use [testids](https://kentcdodds.com/blog/making-your-ui-tests-resilient-to-change/) to select DOM elements
* Since our application is relatively simple, there's no need for Page Objects yet
* [Don't write small tests with single assertions](https://docs.cypress.io/guides/references/best-practices#Creating-tiny-tests-with-a-single-assertion)
* [Mock HTTP requests](https://docs.cypress.io/guides/guides/network-requests#Stub-Responses) when appropriate

