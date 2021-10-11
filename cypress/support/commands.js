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
  capture: 'viewport',
});

// We also overwrite the command, so it does not take a screenshot if we run the tests inside the test runner
Cypress.Commands.overwrite(
  'matchImageSnapshot',
  (originalFn, snapshotName, options) => {
    if (Cypress.env('COMPARE_SNAPSHOTS')) {
      originalFn(snapshotName, options);
    } else {
      cy.log('Screenshot comparison is disabled');
    }
  }
);
