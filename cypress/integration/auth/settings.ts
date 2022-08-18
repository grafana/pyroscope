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

  it.only('should be able to see correct settings page', () => {
    cy.visit('/login');

    cy.get('input#username').focus().type('admin');
    cy.get('input#password').focus().type('admin');
    cy.findByTestId('sign-in-button').click();

    cy.findByTestId('sidebar-settings').click();
    cy.url().should('contain', '/settings');

    cy.findByTestId('settings-userstab').click();

    cy.url().should('contain', '/settings/users');

    // Two users should be displayed
    cy.findByTestId('table-ui').get('tbody>tr').should('have.length', 2);
    cy.findByTestId('table-ui')
      .get('tbody > tr:nth-child(1)')
      .should('contain.text', 'admin@localhost');

    cy.findByTestId('settings-adduser').click();
    cy.url().should('contain', '/settings/users/add');

    cy.get('#userAddName').type('user');
    cy.get('#userAddPassword').type('user');
    cy.get('#userAddEmail').type('user@domain.com');
    cy.get('#userAddFullName').type('Readonly User');
    cy.findByTestId('settings-useradd').click();

    cy.url().should('contain', '/settings/users');

    // Two users should be displayed
    cy.findByTestId('table-ui').get('tbody>tr').should('have.length', 3);
    cy.findByTestId('table-ui')
      .get('tbody>tr:nth-child(3)')
      .should('contain.text', 'user@domain.com');

    cy.visit('/logout');
    cy.visit('/login');

    cy.get('input#username').focus().type('user');
    cy.get('input#password').focus().type('user');
    cy.findByTestId('sign-in-button').click();

    // Expect it to be redirected to main page
    cy.url().should('contain', '/?query=');

    cy.visit('/logout');
  });

  it.only('should be able to change password', () => {
    cy.visit('/login');

    cy.get('input#username').focus().type('user');
    cy.get('input#password').focus().type('user');
    cy.findByTestId('sign-in-button').click();

    cy.findByTestId('sidebar-settings').click();
    cy.url().should('contain', '/settings');

    cy.findByText('Change Password').click();
    cy.url().should('contain', '/settings/security');

    cy.get('input[name="oldPassword"]').focus().type('user');
    cy.get('input[name="password"]').focus().type('pass');
    cy.get('input[name="passwordAgain"]').focus().type('pass');

    cy.get('button').findByText('Save').click();

    cy.findByText('Password has been successfully changed').should(
      'be.visible'
    );

    cy.visit('/logout');
    cy.visit('/login');

    cy.get('input#username').focus().type('user');
    cy.get('input#password').focus().type('pass');
    cy.findByTestId('sign-in-button').click();

    cy.findByTestId('sidebar-settings').click();
    cy.url().should('contain', '/settings');
  });
});
