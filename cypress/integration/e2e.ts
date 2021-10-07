// Following tests should have NO mocking involved.
// The objective involve validating server/webapp interaction is working correctly
import { v4 as uuidv4 } from 'uuid';

describe('E2E Tests', () => {
  // TODO:
  // instead of generating a new application
  // delete the old one?
  let appName = '';
  before(() => {
    appName = uuidv4();

    cy.request({
      method: 'POST',
      url: `/ingest?name=${appName}&sampleRate=100&from=1633613190&until=1633613190`,
      body: 'foo;bar 100',
    });

    cy.request({
      method: 'POST',
      url: `/ingest?name=${appName}&sampleRate=100&from=1633613290&until=1633613290`,
      body: 'foo;bar 10',
    });
  });

  const findFlamegraph = (n: number) => {
    const query = `> :nth-child(${n})`;

    return cy.findByTestId('comparison-container').find(query);
  };

  it('tests single view', () => {
    cy.visit(`/?query=${appName}&from=1633613190&until=1633613190`);

    cy.findByTestId('table-view').matchImageSnapshot(`e2e-single-table`);
    cy.findByTestId('flamegraph-canvas').matchImageSnapshot(
      `e2e-single-flamegraph`
    );
  });

  it('tests /comparison view', () => {
    cy.visit(
      `/comparison?query=${appName}&from=1633613008&leftFrom=1633613177&leftUntil=1633613201&rightFrom=1633613282&rightUntil=1633613304&until=1633613381`
    );

    // flamegraph 1 (the left one)
    findFlamegraph(1)
      .findByTestId('flamegraph-canvas')
      .matchImageSnapshot(`e2e-comparison-flamegraph-left`);

    findFlamegraph(1)
      .findByTestId('table-view')
      .matchImageSnapshot(`e2e-comparison-table-right`);

    findFlamegraph(2)
      .findByTestId('flamegraph-canvas')
      .matchImageSnapshot(`e2e-comparison-flamegraph-right`);

    findFlamegraph(2)
      .findByTestId('table-view')
      .matchImageSnapshot(`e2e-comparison-table-right`);
  });

  it('tests /comparison-diff view', () => {
    cy.visit(
      `/comparison-diff?query=${appName}&from=1633613190&until=1633613190&leftFrom=1633613290&leftUntil=1633613290`
    );

    cy.findByTestId('flamegraph-canvas').matchImageSnapshot(
      `e2e-comparison-diff-flamegraph`
    );

    cy.findByTestId('table-view').matchImageSnapshot(
      `e2e-comparison-diff-table`
    );
  });
});
