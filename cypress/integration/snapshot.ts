
// just testing
it('matches snapshot', () => {
  cy.intercept('**/render*', {
    fixture: 'simple-golang-app-cpu.json',
  }).as('render')

  cy.visit('/');
//  cy.findByTestId('flamegraph-view').toMatchImageSnapshot();

  cy.findByTestId('flamegraph-canvas').matchImageSnapshot();
});
