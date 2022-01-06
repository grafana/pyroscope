import { Result } from '@utils/fp';
import { fetchAppNames } from './appNames';
import { setupServer, rest } from './testUtils';

const server = setupServer(
  rest.get('./label-values', (req, res, ctx) => {
    return res(
      ctx.status(500),
      ctx.json({ message: 'Deliberately broken request' })
    );
    //    return res(
    //      ctx.status(500), ctx.
    //    )
    //    return res(
    //      ctx.json({
    //        title: 'Lord of the Rings',
    //
    //        author: 'J. R. R. Tolkien',
    //      })
    //    );
  })
);

// export const tasksHandlerException = rest.get('test', async (req, res, ctx) =>
//  res(ctx.status(500), ctx.json({ message: 'Deliberately broken request' }))
// );
//
// setupServer([tasksHandlerException]);
//

describe('AppNames', () => {
  beforeAll(() => {
    server.listen();
  });
  afterAll(() => {
    server.close();
  });

  it('fails when server fails', async () => {
    //    setupServer([
    //      rest.get('./label-values?label=__name__', async (req, res, ctx) => {
    //        return res(ctx.status(500));
    //      }),
    //    ]);
    //    fetchMock.mockRejectOnce((req) => {
    //      return Promise.reject(
    //        Error({
    //          ok: false,
    //          statusCode: 500,
    //        })
    //      );
    //    });
    //
    const response = await fetchAppNames();

    expect(true).toBe(true);
    expect(response).toBe(
      Result.err({
        message: 'Response not ok.',
      })
    );
  });
});
