describe('language smoke tests', () => {
  it('works with dotnet', () => {
    cy.intercept('**/render*', {
      fixture: 'cart-service-dotnet-cpu.json',
    }).as('render');

    cy.visit('/');
    cy.findByTestId('flamegraph-canvas').matchImageSnapshot(
      `cart-service-dotnet-cpu-flamegraph`
    );
  });

  it('works with python', () => {
    cy.intercept('**/render*', {
      fixture: 'hotrod-python-driver-cpu.json',
    }).as('render');

    cy.visit('/');
    cy.findByTestId('flamegraph-canvas').matchImageSnapshot(
      `hotrod-python-driver-cpu-flamegraph`
    );
  });

  it('works with ruby', () => {
    cy.intercept('**/render*', {
      fixture: 'hotrod-ruby-driver-cpu.json',
    }).as('render');

    cy.visit('/');
    cy.findByTestId('flamegraph-canvas').matchImageSnapshot(
      `hotrod-ruby-driver-cpu-flamegraph`
    );
  });

  it('works with go', () => {
    cy.intercept('**/render*', {
      fixture: 'shipping-service-go-cpu.json',
    }).as('render');

    cy.visit('/');
    cy.findByTestId('flamegraph-canvas').matchImageSnapshot(
      `shipping-service-go-cpu-flamegraph`
    );
  });
});
