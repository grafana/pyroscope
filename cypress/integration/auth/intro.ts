describe('misc pages', () => {
  it('should correctly display 404 page', () => {
    cy.visit('/404');
    cy.get('h1').should('contain', 'This page does not exist');
  });
});
