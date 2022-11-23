// ***********************************************
// This example commands.js shows you how to
// create various custom commands and overwrite
// existing commands.
//
// For more comprehensive examples of custom
// commands please read more here:
// https://on.cypress.io/custom-commands
// ***********************************************
//
//
// -- This is a parent command --
// Cypress.Commands.add('login', (email, password) => { ... })
//
//
// -- This is a child command --
// Cypress.Commands.add('drag', { prevSubject: 'element'}, (subject, options) => { ... })
//
//
// -- This is a dual command --
// Cypress.Commands.add('dismiss', { prevSubject: 'optional'}, (subject, options) => { ... })
//
//
// -- This will overwrite an existing command --
// Cypress.Commands.overwrite('visit', (originalFn, url, options) => { ... })
import '@testing-library/cypress/add-commands';

import { addMatchImageSnapshotCommand } from 'cypress-image-snapshot/command';

addMatchImageSnapshotCommand({
  failureThreshold: 0.15,
  capture: 'viewport',
});

// We also overwrite the command, so it does not take a screenshot if we run the tests inside the test runner
Cypress.Commands.overwrite(
  'matchImageSnapshot',
  (originalFn, snapshotName, options) => {
    if (Cypress.env('COMPARE_SNAPSHOTS')) {
      // wait a little bit
      // that's to try to avoid blurry screenshots
      // eslint-disable-next-line cypress/no-unnecessary-waiting
      cy.wait(500);
      originalFn(snapshotName, options);
    } else {
      cy.log('Screenshot comparison is disabled');
    }
  }
);

// cy.findByTestId('my-container').get('waitForFlamegraphToRender')
// or
// cy.waitForFlamegraphToRender()
Cypress.Commands.add(
  'waitForFlamegraphToRender',
  { prevSubject: 'optional' },
  ($element) => {
    // it's important to use find/get since the caller requires a DOM element
    if ($element) {
      return cy
        .wrap($element)
        .find('[data-testid="flamegraph-canvas"][data-state="rendered"]');
    }

    return cy.get('[data-testid="flamegraph-canvas"][data-state="rendered"]');
  }
);
