describe('timezone', () => {
  describe('selector', () => {
    it('disabled if local time = UTC', () => {
      const diff = new Date().getTimezoneOffset();
      cy.visit('/');

      cy.findByTestId('time-dropdown-button').click();

      if (diff === 0) {
        cy.get('#select-timezone').should('be.disabled');
      } else {
        cy.get('#select-timezone').should('not.be.disabled');
      }
    });

    it('has correct values', () => {
      cy.visit('/');

      cy.findByTestId('time-dropdown-button').click();

      cy.get('#select-timezone')
        .select(0, { force: true })
        .should('have.value', String(new Date().getTimezoneOffset()));

      cy.get('#select-timezone')
        .select(1, { force: true })
        .should('have.value', '0');
    });

    it('changes what "until"-input renders on setting UTC/local time', () => {
      cy.visit('/');
      cy.findByTestId('time-dropdown-button').click();
      cy.get('#datepicker-until')
        .invoke('val')
        .then((value) => {
          const diff = new Date().getTimezoneOffset();
          cy.get('#select-timezone').select(1, { force: true });

          if (diff !== 0) {
            cy.get('#datepicker-until').should('not.have.value', value);
          } else {
            cy.get('#datepicker-until').should('have.value', value);
          }

          cy.get('#select-timezone').select(0, { force: true });

          cy.get('#datepicker-until').should('have.value', value);
        });
    });
  });
});
