// These tests currently only cover the existence of the main components
// Such as timeline, flamegraph etc
describe('pages', () => {
  it('loads / (single) correctly', () => {
    cy.intercept('**/render*', {
      fixture: 'simple-golang-app-cpu.json',
    }).as('render');

    cy.visit('/');

    cy.findByTestId('flamegraph-canvas');
    cy.findByTestId('timeline-single');
  });

  it.only('loads /comparison (single) correctly', () => {
    cy.intercept('**/render*from=1633024298&until=1633024302*', {
      fixture: 'simple-golang-app-cpu.json',
      times: 1,
    }).as('render-main-timeline');

    cy.intercept('**/render*from=1633024300&until=1633024300*', {
      fixture: 'simple-golang-app-cpu.json',
      times: 1,
    }).as('render-right');

    cy.intercept('**/render*from=1633024290&until=1633024290*', {
      fixture: 'simple-golang-app-cpu2.json',
      times: 1,
    }).as('render-left');

    cy.visit(
      '/comparison?query=simple.golang.app.cpu%7B%7D&from=1633024298&until=1633024302&leftFrom=1633024290&leftUntil=1633024290&rightFrom=1633024300&rightUntil=1633024300'
    );

    cy.wait('@render-right');
    cy.wait('@render-left');
    cy.wait('@render-main-timeline');

    cy.findByTestId('timeline-main');
    cy.findByTestId('timeline-left');
    cy.findByTestId('timeline-right');

    cy.findByTestId('flamegraph-comparison-left');
    cy.findByTestId('flamegraph-comparison-right');
  });
});
