// Following tests should have NO mocking involved.
// The objective involve validating server/webapp interaction is working correctly
// Therefore we can't really validate certain states
//
// TODO
// somehow seed the database?
describe('E2E Tests', () => {
  it('tests single view', () => {
    cy.visit('/');
  });

  it('tests /comparison view', () => {
    cy.visit('/comparison');
  });

  it('tests /comparison-diff view', () => {
    cy.visit('/comparison-diff');
  });
});
