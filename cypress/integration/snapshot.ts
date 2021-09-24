// just testing
it('matches snapshot', () => {
  cy.viewport(1024, 768);

  cy.intercept('**/render*', {
    fixture: 'simple-golang-app-cpu.json',
  }).as('render');

  cy.visit('/');
  //  cy.findByTestId('flamegraph-view').toMatchImageSnapshot();

  cy.findByTestId('flamegraph-canvas').matchImageSnapshot();
});
