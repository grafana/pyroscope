// For these tests we can mock the requests
// Since we are only testing the panel itself
describe('smoke', () => {
  it('renders the panel correctly', () => {
    cy.intercept('**/render*', {
      fixture: 'simple-golang-app-cpu.json',
    }).as('render');

    cy.visit('http://localhost:3000/d/single-panel/pyroscope-demo?orgId=1');

    // Hopefully there should not be any visual changes here
    cy.findByTestId('flamegraph-canvas').matchImageSnapshot(
      'grafana-simple-golang-app-cpu'
    );
  });
});
