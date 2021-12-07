describe('sidebar', () => {
  describe('not collapsed', () => {
    // on smaller screens component will be collapsed by default
    beforeEach(() => {
      cy.viewport(1440, 900);
    });

    it('internal sidebar links work', () => {
      cy.visit('/');

      cy.findByTestId('sidebar-continuous-comparison').click();
      cy.location('pathname').should('eq', '/comparison');

      cy.findByTestId('sidebar-continuous-diff').click();
      cy.location('pathname').should('eq', '/comparison-diff');

      cy.findByTestId('sidebar-continuous-single').click();
      cy.location('pathname').should('eq', '/');
    });
  });
});
