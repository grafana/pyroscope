describe('service discovery page', () => {
  it('works', () => {
    const basePath = Cypress.env('basePath') || '';
    cy.intercept(`${basePath}/targets`, {
      fixture: 'targets.json',
    }).as('targets');

    cy.visit('/service-discovery');
    cy.wait('@targets');

    // one for the header and another for the content
    cy.findAllByRole('row').should('have.length', 2);

    cy.findByText('http://nodejs:3000/debug/pprof/profile?seconds=10');
  });
});
