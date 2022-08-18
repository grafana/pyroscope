describe('QueryParams', () => {
  // on smaller screens component will be collapsed by default
  beforeEach(() => {
    cy.viewport(1440, 900);
  });

  // type a tag so that it's synced to the URL
  // IMPORTANT! don't access the url directly since it will render this test useless
  // since we are testing populating the queryParams and maintaining between routes
  it('maintains queryParams when changing route', () => {
    const myTag = 'myrandomtag{}';
    const validate = () => {
      cy.location().then((loc) => {
        const urlParams = new URLSearchParams(loc.search);
        expect(urlParams.get('query')).to.eq(myTag);
      });
    };

    cy.visit('/');
    cy.get(`[aria-label="query-input"] textarea`).clear().type(myTag);
    cy.get(`[aria-label="query-input"] button`).click();
    validate();

    cy.findByTestId('sidebar-continuous-comparison').click();
    validate();

    cy.findByTestId('sidebar-continuous-diff').click();
    validate();

    cy.findByTestId('sidebar-continuous-single').click();
    validate();

    cy.findByTestId('sidebar-explore-page').click();
    validate();
  });
});
