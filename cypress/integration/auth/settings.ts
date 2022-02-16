// few tests just to quickly validate the endpoints are working
describe('Settings page', () => {
  it('should display error when log in with random creds', () => {
    cy.visit('/login');

    cy.get('input#username').focus().type('random');
    cy.get('input#password').focus().type('user');
    cy.get('button.sign-in-button').click();
    cy.get('#error').should('contain.text', 'invalid credentials');
    // Expect it not to be redirected to main page
    cy.url().should('contain', '/login');
  });

  it('should be able to log in with default creds', () => {
    cy.visit('/login');

    cy.get('input#username').focus().type('admin');
    cy.get('input#password').focus().type('admin');
    cy.get('button.sign-in-button').click();

    // Expect it to be redirected to main page
    cy.url().should('contain', '/?query=');

    cy.visit('/logout');
  });

  it('should be able to see correct settings page', () => {
    cy.visit('/login');

    cy.get('input#username').focus().type('admin');
    cy.get('input#password').focus().type('admin');
    cy.get('button.sign-in-button').click();

    cy.findByTestId('sidebar-settings').click();
    cy.url().should('contain', '/settings');

    cy.visit('/logout');
  });
});
