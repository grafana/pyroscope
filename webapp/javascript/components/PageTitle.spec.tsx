import React from 'react';
import PageTitle, { AppNameContext } from './PageTitle';
import { render, screen, waitFor } from '@testing-library/react';

describe('PageTitle', () => {
  describe("there's no app name in context", () => {
    it('defaults to Pyroscope', async () => {
      render(<PageTitle title={'mypage'} />);

      await waitFor(() => expect(document.title).toEqual('mypage | Pyroscope'));
    });
  });

  describe("there's an app name in context", () => {
    it('suffixes the title with it', async () => {
      render(
        <AppNameContext.Provider value={'myapp'}>
          <PageTitle title={'mypage'} />
        </AppNameContext.Provider>
      );

      await waitFor(() => expect(document.title).toEqual('mypage | myapp'));
    });
  });
});
