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


  it('app selector works', () => {

    cy.intercept('**/render*', {
      fixture: 'render.json',
      times: 1
    }).as('render1')

    cy.visit('/');

    cy.wait('@render1');

    cy.fixture('render.json').then((data) => {
      cy.findByTestId('table-view').contains('td', data.flamebearer.names[0]).should('be.visible');
      cy.findByTestId('table-view').contains('td', data.flamebearer.names[data.flamebearer.names.length - 1]).should('be.visible');
    });

    cy.intercept('**/render*', {
      fixture: 'render2.json',
      times: 1
    }).as('render2')
    
    cy.findByTestId('app-name-selector').select('pyroscope.server.cpu');
    
    cy.wait('@render2');

    cy.fixture('render2.json').then((data) => {
      cy.findByTestId('table-view').contains('td', data.flamebearer.names[0]).should('be.visible');
      cy.findByTestId('table-view').contains('td', data.flamebearer.names[data.flamebearer.names.length - 1]).should('be.visible');
    });

  });

  it('updates flamegraph on app name change', () => {
    cy.visit('/')

    cy.findByTestId('app-name-selector').select('pyroscope.server.cpu');
    cy.findByTestId('flamegraph-canvas').invoke('attr', 'data-appname').should('eq', 'pyroscope.server.cpu{}');
  });

  it('view buttons should change view when clicked', () => {
    cy.visit('/')
    cy.findByTestId('btn-table-view').click();
    cy.findByTestId('table-view').should('be.visible');
    cy.findByTestId('flamegraph-view').should('not.exist');
        
    cy.findByTestId('btn-both-view').click();
    cy.findByTestId('table-view').should('be.visible');
    cy.findByTestId('flamegraph-view').should('be.visible');
      
    cy.findByTestId('btn-flamegraph-view').click();
    cy.findByTestId('table-view').should('not.be.visible');
    cy.findByTestId('flamegraph-view').should('be.visible');
  });

  it('sorting is working', () => {        
    /** 
     * @param row 'first' | 'last' 
     * @param column 'location' | 'self' | 'total'
    */

    const columns = {
      location: {
        index: 1,
        selector: '.symbol-name'
      },
      self: {
        index: 2,
        selector: 'span'
      },
      total: {
        index: 3,
        selector: 'span'
      }
    }

    const sortColumn = (columnIndex) => {
      return cy.findByTestId('table-view').find(`thead > tr > :nth-child(${columnIndex})`).click();
    }

    const getCellContent =  (row, column) => {
        let query = `tbody > :nth-child(${row}) > :nth-child(${column.index}) > ${column.selector}`;
        return cy.findByTestId('table-view').find(query)
        .then(cell => cell[0].innerText);
    }

    cy.intercept('**/render*', {
      fixture: 'render.json',
      times: 1
    }).as('render');

    cy.visit('/');

    cy.findByTestId('table-view')
      .find('tbody > tr')
      .then((rows) => {
        const first = 1;
        const last = rows.length;

        //sort by location desc
        sortColumn(columns['location'].index);
        getCellContent(first, columns['location']).should('eq', 'function_6');
        getCellContent(last, columns['location']).should('eq', 'function_0');
        
        //sort by location asc
        sortColumn(columns['location'].index);
        getCellContent(first, columns['location']).should('eq', 'function_0');
        getCellContent(last, columns['location']).should('eq', 'function_6');

        //sort by self desc
        sortColumn(columns['self'].index);
        getCellContent(first, columns['self']).should('eq', '5.00 seconds');
        getCellContent(last, columns['self']).should('eq', '0.55 seconds');

        //sort by self asc
        sortColumn(columns['self'].index);
        getCellContent(first, columns['self']).should('eq', '0.55 seconds');
        getCellContent(last, columns['self']).should('eq', '5.00 seconds');

        //sort by total desc
        sortColumn(columns['total'].index);
        getCellContent(first, columns['total']).should('eq', '5.16 seconds');
        getCellContent(last, columns['total']).should('eq', '0.50 seconds');

        //sort by total asc
        sortColumn(columns['total'].index);
        getCellContent(first, columns['total']).should('eq', '0.50 seconds');
        getCellContent(last, columns['total']).should('eq', '5.16 seconds');
      })
  });

  it('validates "Reset View" works', () => {
    cy.intercept('**/render*', {
      fixture: 'simple-golang-app-cpu.json',
    }).as('render')

    cy.visit('/')

    cy.findByTestId('reset-view').should('not.be.visible');
    cy.findByTestId('flamegraph-canvas')
      .click(0, BAR_HEIGHT)
    cy.findByTestId('reset-view').should('be.visible');
    cy.findByTestId('reset-view').click();
    cy.findByTestId('reset-view').should('not.be.visible');
  });
})
    
