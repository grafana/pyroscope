// few tests just to quickly validate the endpoints are working
describe('unauth API tests', () => {
  it('it should respond with 401 on unauthorized access', () => {
    cy.request({
      method: 'GET',
      url: '/render?from=now-5m&until=now&query=pyroscope.server.alloc_objects%7B%7D&format=json',
      failOnStatusCode: false,
    })
      .its('status')
      .should('eq', 401);
  });

  it('it should respond with 200 on login and signup', () => {
    cy.request({
      method: 'GET',
      url: '/login',
      failOnStatusCode: false,
    })
      .its('status')
      .should('eq', 200);

    cy.request({
      method: 'GET',
      url: '/signup',
      failOnStatusCode: false,
    })
      .its('status')
      .should('eq', 200);
  });
});
