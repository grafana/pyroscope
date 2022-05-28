describe('misc pages', () => {
  it('should correctly display 404 page', () => {
    cy.visit('/404', { failOnStatusCode: false });
    cy.get('h1').should('contain', 'This page does not exist');
  });

  it('should redirect back to requested page after logging in', () => {
    cy.visit('/comparison');
    // it should redirect back to login
    cy.url().should('contain', '/login');
    // Enter credentials
    cy.get('input#username').focus().type('admin');
    cy.get('input#password').focus().type('admin');
    cy.get('button.sign-in-button').click();

    cy.url().should('contain', '/comparison');
  });
});
