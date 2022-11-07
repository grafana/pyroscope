describe('oauth with mock enabled', () => {
  beforeEach(() => {
    cy.clearCookies();
  });
  it('should correctly display buttons on login page', () => {
    cy.visit('/login');

    cy.get('#gitlab-link').should('be.visible');
    cy.get('#google-link').should('not.exist');
    cy.get('#github-link').should('not.exist');

    cy.get('#gitlab-link').click();

    // When accessing /login directly we should be redirected to the root
    cy.location().should((loc) => {
      const removeTrailingSlash = (url: string) => url.replace(/\/+$/, '');

      const basePath = new URL(Cypress.config().baseUrl).pathname;

      expect(removeTrailingSlash(loc.pathname)).to.eq(
        removeTrailingSlash(basePath)
      );
    });

    cy.intercept('/api/user');

    cy.findByTestId('sidebar-settings').click();

    cy.findByText('Change Password').should('not.exist');

    cy.get('li.pro-menu-item').contains('Sign out').click({ force: true });
    cy.url().should('contain', '/login');
  });
});
