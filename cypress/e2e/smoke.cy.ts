// / <reference types="cypress" />
describe('smoke', () => {
  beforeEach(function () {
    const apiBasePath = Cypress.env('apiBasePath') || '';

    cy.intercept(`${apiBasePath}/querier.v1.QuerierService/LabelNames`, {
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

  it('changes path when navigating', () => {
    const clickSidebar = (name: string) => {
      cy.get('nav.pro-menu .pro-item-content')
        .filter(`:contains("${name}")`)
        .parent()
        .parent()
        .click();
    };

    cy.visit('/');
    cy.url().should('contain', '/');
    cy.title().should('include', 'Single');

    clickSidebar('Comparison View');
    cy.url().should('contain', '/comparison');
    cy.title().should('include', 'Comparison');

    clickSidebar('Diff View');
    cy.url().should('contain', '/comparison-diff');
    cy.title().should('include', 'Diff');

    clickSidebar('Tag Explorer');
    cy.url().should('contain', '/explore');
    cy.title().should('include', 'Tag Explorer');

    clickSidebar('Single');
    cy.url().should('contain', '/');
    cy.title().should('include', 'Single');
  });
});
