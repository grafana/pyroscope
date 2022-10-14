describe('Annotations', () => {
  it('add annotation flow works as expected', () => {
    const basePath = Cypress.env('basePath') || '';
    cy.intercept(`${basePath}**/label-values**`).as('labelValues');

    cy.visit('/');

    cy.wait('@labelValues').wait(1000);

    cy.get('canvas.flot-overlay').click();

    cy.get('li[role=menuitem]').contains('Add annotation').click();

    const content = 'this is annotation content';
    let time;

    cy.get('form#annotation-form')
      .findByTestId('annotation_timestamp_input')
      .invoke('val')
      .then((sometext) => (time = sometext));

    cy.get('form#annotation-form')
      .findByTestId('annotation_content_input')
      .type(content);

    cy.get('button[form=annotation-form]').click();

    cy.findByTestId('annotation_mark_wrapper').click();

    cy.get('form#annotation-form')
      .findByTestId('annotation_content_input')
      .should('have.value', content);

    cy.get('form#annotation-form')
      .findByTestId('annotation_timestamp_input')
      .invoke('val')
      .then((sometext2) => assert.isTrue(sometext2 === time));

    cy.get('button[form=annotation-form]').contains('Close').click();

    cy.get('form#annotation-form').should('not.exist');
  });
});
