/// <reference types="cypress" />
describe('basic test', () => {
  it('successfully loads', () => {
    cy.visit('/')
    cy.title().should('eq', 'Pyroscope');
  });

  it('internal sidebar links work', () => {
    cy.visit('/')

    waitInDevMode(100);

    cy.findByTestId('sidebar-comparison').click();
    waitInDevMode(100);
    cy.location('pathname').should('eq', '/comparison');

    cy.findByTestId('sidebar-comparison-diff').click();
    waitInDevMode(100);
    cy.location('pathname').should('eq', '/comparison-diff');

    cy.findByTestId('sidebar-root').click();
    waitInDevMode(100);
    cy.location('pathname').should('eq', '/');
  });
})

// very nasty, just to avoid dealing with the following error
// which requires aborting fetch call and whatnot
// react-dom.development.js:21 Warning: Can't perform a React state update on an unmounted component. This is a no-op, but it indicates a memory leak in your application. To fix, cancel all subscriptions and asynchronous tasks in the componentWillUnmount method.
// in FlameGraphRenderer (created by Context.Consumer)
// in e (created by ConnectFunction)
// in ConnectFunction (created by PyroscopeApp)
// in div (created by PyroscopeApp)
// in div (created by PyroscopeApp)
// in PyroscopeApp (created by ConnectFunction)
// in ConnectFunction
function waitInDevMode(t: number) {
  if (!process.env.CI) {
    cy.wait(t);
  }
}
