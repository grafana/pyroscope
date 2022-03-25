import { Result } from '@webapp/util/fp';
import { fetchAppNames } from './appNames';
import { setupServer, rest } from './testUtils';

describe('AppNames', () => {
  let server: ReturnType<typeof setupServer> | null;

  afterEach(() => {
    if (server) {
      server.close();
    }
    server = null;
  });

  it('fetches appNames', async () => {
    server = setupServer(
      rest.get(`http://localhost/label-values`, (req, res, ctx) => {
        // TODO check query params
        //
        return res(
          ctx.status(200),

          ctx.json([
            'pyroscope.server.alloc_objects',
            'pyroscope.server.alloc_space',
            'pyroscope.server.cpu',
            'pyroscope.server.inuse_objects',
            'pyroscope.server.inuse_space',
          ])
        );
      })
    );

    server.listen();
    const res = await fetchAppNames();

    expect(res).toMatchObject(
      Result.ok([
        'pyroscope.server.alloc_objects',
        'pyroscope.server.alloc_space',
        'pyroscope.server.cpu',
        'pyroscope.server.inuse_objects',
        'pyroscope.server.inuse_space',
      ])
    );
  });

  it('ignores apps with invalid names', async () => {
    server = setupServer(
      rest.get(`http://localhost/label-values`, (req, res, ctx) => {
        return res(
          ctx.status(200),

          ctx.json(['app1', 'app2', ' ', ''])
        );
      })
    );

    server.listen();
    const res = await fetchAppNames();

    expect(res).toMatchObject(Result.ok(['app1', 'app2']));
  });
});
