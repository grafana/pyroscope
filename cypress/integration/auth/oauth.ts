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
    cy.url().should('contain', '/?query=');
    // Wait before data load
    cy.waitForFlamegraphToRender();
    // cy.get('.spinner-container.loading').should('be.visible');
    // cy.get('.spinner-container.loading').should('exist');
    cy.get('.spinner-container').should('exist');
    cy.intercept('/api/user');

    cy.findByTestId('sidebar-settings').click();

    cy.findByText('Change Password').should('not.exist');

    cy.get('li.pro-menu-item').contains('Sign out').click({ force: true });
    cy.url().should('contain', '/login');
  });

  it('should correctly display forbidden page', () => {
    cy.visit('/login');

    cy.get('#gitlab-link').should('be.visible');

    cy.get('#gitlab-link').click();
    cy.url().should('contain', '/forbidden');
    cy.visit('/logout');
    cy.url().should('contain', '/login');
  });
});
