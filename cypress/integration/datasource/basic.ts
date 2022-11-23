describe('smoke', () => {
  // Since we are testing the datasource plugin only
  // We don't do any assertions against the panel
  it('makes requests to the datasource', () => {
    cy.intercept('**/render*', {
      fixture: 'simple-golang-app-cpu.json',
    }).as('render');

    cy.visit('http://localhost:3000/d/single-panel/pyroscope-demo?orgId=1');

    cy.intercept(
      'http://localhost:3000/api/datasources/proxy/1/render/render?format=json&from=now-5m&until=now&queryType=randomWalk&refId=A&datasource=Pyroscope&query=pyroscope.server.cpu'
    ).as('renderRequest');

    cy.wait('@renderRequest');
  });
});
