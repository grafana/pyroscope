// / <reference types="cypress" />
describe('smoke', () => {
  beforeEach(function () {
    const apiBasePath = Cypress.env('apiBasePath') || '';

    cy.intercept(`${apiBasePath}/pyroscope/label-values?label=__name__`, {
      fixture: 'profileTypes.json',
    }).as('profileTypes');
  });

  it('loads admin page', () => {
    cy.visit('../');
  });

  it('loads single view (/)', () => {
    cy.visit('/');
    cy.wait(`@profileTypes`);
  });

  it('loads comparison view (/comparison)', () => {
    cy.visit('/comparison');
    cy.wait(`@profileTypes`);
  });

  it('loads diff view (/comparison-diff)', () => {
    cy.visit('/comparison-diff');
    cy.wait(`@profileTypes`);
  });
});
