describe('sidebar', () => {
  describe('not collapsed', () => {
    // on smaller screens component will be collapsed by default
    beforeEach(() => {
      cy.viewport(1440, 900);
    });

    it('internal sidebar links work', () => {
      cy.visit('/');

      cy.findByTestId('sidebar-continuous-comparison').click();

      const basePath = Cypress.env('basePath') || '';
      cy.location('pathname').should('eq', `${basePath}/comparison`);

      cy.findByTestId('sidebar-continuous-diff').click();
      cy.location('pathname').should('eq', `${basePath}/comparison-diff`);

      cy.findByTestId('sidebar-continuous-single').click();
      cy.location('pathname').should('eq', `${basePath}/`);
    });
  });

  describe('persists', () => {
    beforeEach(() => {
      cy.viewport(1440, 900);
    });
    it('should have sidebar', () => {
      cy.visit('/');

      cy.get('.app').find('.pro-sidebar').should('not.have.class', 'collapsed');
      cy.get('.app')
        .find('.pro-sidebar')
        .findByText('Collapse Sidebar')
        .click();

      cy.get('.app').find('.pro-sidebar').should('have.class', 'collapsed');

      cy.reload();

      cy.get('.app').find('.pro-sidebar').should('have.class', 'collapsed');
    });
  });
});
