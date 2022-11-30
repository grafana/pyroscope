const BAR_HEIGHT = 21.5;

// / <reference types="cypress" />
describe('basic test', () => {
  beforeEach(function () {
    const basePath = Cypress.env('basePath') || '';

    cy.intercept(`${basePath}/api/apps`, {
      fixture: 'appNames.json',
    }).as('appNames');
  });

  it('changes app via the application dropdown', () => {
    cy.visit('/');
    cy.wait(`@appNames`);

    cy.get('.navbar').findAllByTestId('toggler').click();

    // For some reason couldn't find the appropriate query
    cy.findAllByRole('menuitem').then((items) => {
      items.each((i, item) => {
        if (item.innerText.includes('pyroscope.server.inuse_space')) {
          item.click();
        }
      });
    });

    cy.location().then((loc) => {
      const queryParams = new URLSearchParams(loc.search);
      expect(queryParams.get('query')).to.eq('pyroscope.server.inuse_space{}');
    });
  });

  it('view buttons should change view when clicked', () => {
    // mock data since the first preselected application
    // could have no data
    cy.intercept('**/render*', {
      fixture: 'simple-golang-app-cpu.json',
      times: 1,
    }).as('render1');

    cy.visit('/');

    cy.findByTestId('table').click();
    cy.findByTestId('table-ui').should('be.visible');
    cy.findByTestId('flamegraph-view').should('not.exist');

    cy.findByTestId('both').click();
    cy.findByTestId('table-ui').should('be.visible');
    cy.findByTestId('flamegraph-view').should('be.visible');

    cy.findByTestId('flamegraph').click();
    cy.findByTestId('table-ui').should('not.exist');
    cy.findByTestId('flamegraph-view').should('be.visible');
  });

  // TODO make this a unit test
  it('sorting works', () => {
    /**
     * @param row 'first' | 'last'
     * @param column 'location' | 'self' | 'total'
     */

    const columns = {
      location: {
        index: 1,
        selector: '.symbol-name',
      },
      self: {
        index: 2,
        selector: 'span',
      },
      total: {
        index: 3,
        selector: 'span',
      },
    };

    const sortColumn = (columnIndex) =>
      cy
        .findByTestId('table-ui')
        .find(`thead > tr > :nth-child(${columnIndex})`)
        .click();

    const getCellContent = (row, column) => {
      const query = `tbody > :nth-child(${row}) > :nth-child(${column.index})`;
      return cy
        .findByTestId('table-ui')
        .find(query)
        .then((cell) => cell[0].innerText);
    };

    cy.intercept('**/render*', {
      fixture: 'render.json',
      times: 1,
    }).as('render');

    cy.visit('/');

    cy.findByTestId('table-ui')
      .find('tbody > tr')
      .then((rows) => {
        const first = 1;
        const last = rows.length;

        // sort by location desc
        sortColumn(columns.location.index);
        getCellContent(first, columns.location).should('eq', 'function_6');
        getCellContent(last, columns.location).should('eq', 'function_0');

        // sort by location asc
        sortColumn(columns.location.index);
        getCellContent(first, columns.location).should('eq', 'function_0');
        getCellContent(last, columns.location).should('eq', 'function_6');

        // sort by self desc
        sortColumn(columns.self.index);
        getCellContent(first, columns.self).should('eq', '5.00 seconds');
        getCellContent(last, columns.self).should('eq', '0.55 seconds');

        // sort by self asc
        sortColumn(columns.self.index);
        getCellContent(first, columns.self).should('eq', '0.55 seconds');
        getCellContent(last, columns.self).should('eq', '5.00 seconds');

        // sort by total desc
        sortColumn(columns.total.index);
        getCellContent(first, columns.total).should('eq', '5.16 seconds');
        getCellContent(last, columns.total).should('eq', '0.50 seconds');

        // sort by total asc
        sortColumn(columns.total.index);
        getCellContent(first, columns.total).should('eq', '0.50 seconds');
        getCellContent(last, columns.total).should('eq', '5.16 seconds');
      });
  });

  it('validates "Reset View" button works', () => {
    cy.intercept('**/render*', {
      fixture: 'simple-golang-app-cpu.json',
    }).as('render');

    cy.visit('/');

    cy.findByRole('button', { name: /Reset/ }).should('be.disabled');
    cy.waitForFlamegraphToRender().click(0, BAR_HEIGHT * 2);
    cy.findByRole('button', { name: /Reset/ }).should('not.be.disabled');
    cy.findByRole('button', { name: /Reset/ }).click();
    cy.findByRole('button', { name: /Reset/ }).should('be.disabled');
  });

  describe('tooltip', () => {
    // on smaller screens component will be collapsed by default
    beforeEach(() => {
      cy.viewport(1440, 900);
    });
    it('flamegraph tooltip works in single view', () => {
      cy.intercept('**/render*', {
        fixture: 'simple-golang-app-cpu.json',
      }).as('render');

      cy.visit('/');

      cy.findAllByTestId('tooltip').should('not.be.visible');

      cy.waitForFlamegraphToRender().trigger('mousemove');

      cy.findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('be.visible');

      cy.findByTestId('tooltip-title').should('have.text', 'total');
      cy.findByTestId('tooltip-table').should(
        'have.text',
        'Share of CPU:100%CPU Time:9.88 secondsSamples:988'
      );

      cy.waitForFlamegraphToRender().trigger('mouseout');
      cy.findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('not.be.visible');
    });

    it('flamegraph tooltip works in comparison view', () => {
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
      cy.log('left flamegraph');
      findFlamegraph(1)
        .findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('not.be.visible');

      findFlamegraph(1).waitForFlamegraphToRender().trigger('mousemove');

      findFlamegraph(1)
        .findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('have.css', 'visibility', 'visible');

      findFlamegraph(1)
        .findByTestId('tooltip-title')
        .should('have.text', 'total');
      findFlamegraph(1)
        .findByTestId('tooltip-table')
        .should(
          'have.text',
          'Share of CPU:100%CPU Time:9.91 secondsSamples:991'
        );

      findFlamegraph(1).waitForFlamegraphToRender().trigger('mousemove');
      findFlamegraph(1).waitForFlamegraphToRender().trigger('mouseout');

      findFlamegraph(1)
        .findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('not.be.visible');

      // flamegraph 2 (right one)
      cy.log('right flamegraph');
      findFlamegraph(2)
        .findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('not.be.visible');

      findFlamegraph(2).waitForFlamegraphToRender().trigger('mousemove', 0, 0, {
        force: true,
      });

      findFlamegraph(2)
        .findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('have.css', 'visibility', 'visible');

      findFlamegraph(2)
        .findByTestId('tooltip-title')
        .should('have.text', 'total');
      findFlamegraph(2)
        .findByTestId('tooltip-table')
        .should(
          'have.text',
          'Share of CPU:100%CPU Time:9.88 secondsSamples:988'
        );

      findFlamegraph(2)
        .waitForFlamegraphToRender()
        .trigger('mouseout', { force: true });
      findFlamegraph(2)
        .findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('not.be.visible');
    });

    it('flamegraph tooltip works in diff view', () => {
      cy.intercept('**/render*', {
        fixture: 'simple-golang-app-cpu-diff.json',
        times: 3,
      }).as('render');

      cy.visit(
        '/comparison-diff?query=testapp%7B%7D&rightQuery=testapp%7B%7D&leftQuery=testapp%7B%7D&leftFrom=1&leftUntil=1&rightFrom=1&rightUntil=1&from=now-5m'
      );

      cy.wait('@render');
      cy.wait('@render');
      cy.wait('@render');

      cy.waitForFlamegraphToRender();

      // This test has a race condition, since it does not wait for the canvas to be rendered
      cy.findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('not.be.visible');

      cy.waitForFlamegraphToRender().trigger('mousemove', 0, 0);
      cy.findByTestId('flamegraph-view')
        .findByTestId('tooltip')
        .should('have.css', 'visibility', 'visible');

      cy.findByTestId('tooltip-title').should('have.text', 'total');
      cy.findByTestId('tooltip-table').should(
        'have.text',
        'BaselineComparisonDiffShare of CPU:100%100%CPU Time:9.91 seconds9.87 secondsSamples:991987'
      );
    });

    it('table tooltip works in single view', () => {
      cy.intercept('**/render*', {
        fixture: 'simple-golang-app-cpu.json',
        times: 3,
      }).as('render');

      cy.visit('/');

      cy.wait('@render');

      cy.findByTestId('table-view')
        .findByTestId('tooltip')
        .should('not.be.visible');

      cy.findByTestId('table-view').trigger('mousemove', 150, 80);
      cy.findByTestId('table-view')
        .findByTestId('tooltip')
        .should('have.css', 'visibility', 'visible');

      cy.findByTestId('tooltip-table').should(
        'have.text',
        'Self (% of total CPU)Total (% of total CPU)CPU Time:0.02 seconds(0.20%)0.03 seconds(0.30%)'
      );
    });

    it('table tooltip works in diff view', () => {
      cy.intercept('**/render*', {
        fixture: 'simple-golang-app-cpu-diff.json',
        times: 3,
      }).as('render');

      cy.visit(
        '/comparison-diff?query=testapp%7B%7D&rightQuery=testapp%7B%7D&leftQuery=testapp%7B%7D&leftFrom=1&leftUntil=1&rightFrom=1&rightUntil=1&from=now-5m'
      );

      cy.wait('@render');

      cy.findByTestId('table-view')
        .findByTestId('tooltip')
        .should('not.be.visible');

      cy.findByTestId('table-view').trigger('mousemove', 150, 80);
      cy.findByTestId('table-view')
        .findByTestId('tooltip')
        .should('have.css', 'visibility', 'visible');

      cy.findByTestId('tooltip-table').should(
        'have.text',
        'BaselineComparisonDiffShare of CPU:100%99.8%(-0.20%)CPU Time:9.91 seconds9.85 secondsSamples:991985'
      );
    });
  });

  describe('highlight', () => {
    it('works in diff view', () => {
      cy.intercept('**/render*', {
        fixture: 'simple-golang-app-cpu-diff.json',
        times: 1,
      }).as('render');

      cy.visit(
        '/comparison-diff?query=simple.golang.app.cpu%7B%7D&from=1633024298&until=1633024302&leftFrom=1633024290&leftUntil=1633024290&rightFrom=1633024300&rightUntil=1633024300'
      );

      cy.wait('@render');

      cy.findByTestId('flamegraph-highlight').should('not.be.visible');

      cy.wait(500);

      cy.waitForFlamegraphToRender().trigger('mousemove', 0, 0);
      cy.findByTestId('flamegraph-highlight').should('be.visible');
    });
  });

  describe('contextmenu', () => {
    it("it works when 'clear view' is clicked", () => {
      cy.intercept('**/render*', {
        fixture: 'simple-golang-app-cpu.json',
        times: 1,
      }).as('render');

      cy.visit('/');

      // until we focus on a specific, it should not be enabled
      cy.waitForFlamegraphToRender().rightclick();
      cy.findByRole('menuitem', { name: /Reset View/ }).should(
        'have.attr',
        'aria-disabled',
        'true'
      );

      // click on the second item
      cy.waitForFlamegraphToRender().click(0, BAR_HEIGHT * 2);
      cy.waitForFlamegraphToRender().rightclick();
      cy.findByRole('menuitem', { name: /Reset View/ }).should(
        'not.have.attr',
        'aria-disabled'
      );
      cy.findByRole('menuitem', { name: /Reset View/ }).click();
      // TODO assert that it was indeed reset?

      // should be disabled again
      cy.waitForFlamegraphToRender().rightclick();
      cy.findByRole('menuitem', { name: /Reset View/ }).should(
        'have.attr',
        'aria-disabled',
        'true'
      );
    });
  });
});
