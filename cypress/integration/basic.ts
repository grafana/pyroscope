/// <reference types="cypress" />
describe('home page', () => {
  it('successfully loads', () => {
    cy.visit('http://localhost:4040')
    cy.title().should('eq', 'Pyroscope');
  })
})
