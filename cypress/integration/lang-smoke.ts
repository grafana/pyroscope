describe('language smoke tests', () => {
  describe('single view', () => {
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

  describe('comparison', () => {
    it('works with go', () => {
      const findFlamegraph = (n: number) => {
        const query = `> :nth-child(${n})`;

        return cy.findByTestId('comparison-container').find(query);
      };
      cy.intercept('**/render*from=1633024300&until=1633024300*', {
        fixture: 'simple-golang-app-cpu.json',
        times: 1,
      }).as('render-right');

      cy.intercept('**/render*from=1633024290&until=1633024290*', {
        fixture: 'simple-golang-app-cpu2.json',
        times: 1,
      }).as('render-left');
      cy.visit(
        '/comparison?query=simple.golang.app.cpu%7B%7D&from=1633024298&until=1633024302&leftFrom=1633024290&leftUntil=1633024290&rightFrom=1633024300&rightUntil=1633024300'
      );
      cy.wait('@render-right');
      cy.wait('@render-left');

      // flamegraph 1 (the left one)
      findFlamegraph(1)
        .findByTestId('flamegraph-canvas')
        .matchImageSnapshot(`comparison-go-1`);

      // flamegraph 2 (the right one)
      findFlamegraph(2)
        .findByTestId('flamegraph-canvas')
        .matchImageSnapshot(`comparison-go-2`);
    });
  });
});
