// few tests just to quickly validate the endpoints are working
describe('API tests', () => {
  it('tests /render endpoint', () => {
    cy.request(
      'GET',
      '/render?from=now-5m&until=now&query=pyroscope.server.alloc_objects%7B%7D&format=json'
    )
      .its('headers')
      .its('content-type')
      .should('include', 'application/json');
  });

  it('tests /labels endpoint', () => {
    // TODO
    // this is not returning json
    cy.request('GET', '/labels?query=pyroscope.server.alloc_objects');
    //      .its('headers')
    //      .its('content-type')
    //      .should('include', 'application/json');
  });

  it('tests /render-diff endpoint', () => {
    cy.request(
      'GET',
      'http://localhost:4040/comparison-diff?query=pyroscope.server.cpu%7B%7D&rightQuery=pyroscope.server.cpu%7B%7D&leftQuery=pyroscope.server.cpu%7B%7D&leftFrom=1648154123&leftUntil=1648154128&rightFrom=1648154123&rightUntil=1648154129&from=1648154091&until=1648154131'
    );
  });

  it('tests 404 custom page', () => {
    cy.request({ url: '/my-404-page', failOnStatusCode: false })
      .its('status')
      .should('equal', 404);

    cy.visit({ url: '/my-404-page', failOnStatusCode: false });
    cy.get('.pyroscope-app').should('contain.text', 'does not exist');
  });
});
