/// <reference types="cypress" />
describe('basic test', () => {
  it('successfully loads', () => {
    cy.visit('/')
    cy.title().should('eq', 'Pyroscope');
  });

  it('internal sidebar links work', () => {
    cy.visit('/')

    cy.findByTestId('sidebar-comparison').click();
    cy.location('pathname').should('eq', '/comparison');

    cy.findByTestId('sidebar-comparison-diff').click();
    cy.location('pathname').should('eq', '/comparison-diff');

    cy.findByTestId('sidebar-root').click();
    cy.location('pathname').should('eq', '/');
  });

  it('view buttons should change view when clicked', () => {
    cy.visit('/')
    cy.findByTestId('btn-table-view').click();
    cy.findByTestId('table-view').should('be.visible');
    cy.findByTestId('flamegraph-view').should('not.be.visible');
        
    cy.findByTestId('btn-both-view').click();
    cy.findByTestId('table-view').should('be.visible');
    cy.findByTestId('flamegraph-view').should('be.visible');
      
    cy.findByTestId('btn-flamegraph-view').click();
    cy.findByTestId('table-view').should('not.be.visible');
    cy.findByTestId('flamegraph-view').should('be.visible');
  });

})
