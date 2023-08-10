import React from 'react';
import { render, waitFor } from '@testing-library/react';
import PageTitle, { AppNameContext } from './PageTitle';

// Default is 1000, try a few more times since it was failing in ci
const waitForOpts = {
  timeout: 5000,
};

describe('PageTitle', () => {
  describe("there's no app name in context", () => {
    it('defaults to Pyroscope', async () => {
      render(<PageTitle title="mypage" />);

      await waitFor(
        () => expect(document.title).toEqual('mypage | Pyroscope'),
        waitForOpts
      );
    });
  });

  describe("there's an app name in context", () => {
    it('suffixes the title with it', async () => {
      render(
        <AppNameContext.Provider value="myapp">
          <PageTitle title="mypage" />
        </AppNameContext.Provider>
      );

      await waitFor(
        () => expect(document.title).toEqual('mypage | myapp'),
        waitForOpts
      );
    });
  });
});
