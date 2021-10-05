// few tests just to quickly validate the endpoints are working
describe('API tests', () => {
  it('tests /render endpoint', () => {
    cy.request(
      'GET',
      '/render?from=now-5m&until=now&query=pyroscope.server.alloc_objects%7B%7D&format=json',
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
      '/render-diff?from=now-5m&until=now&query=pyroscope.server.alloc_objects{}&max-nodes=1024&leftFrom=now-1h&leftUntil=now-30m&rightFrom=now-30m&rightUntil=now&format=json',
    );
  });
});
