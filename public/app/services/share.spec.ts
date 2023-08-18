import { Result } from '@pyroscope/util/fp';
import { ZodError } from 'zod';
import { shareWithFlamegraphDotcom } from './share';
import { setupServer, rest } from './testUtils';
// TODO move this testData to somewhere else
import TestData from './TestData';

describe('Share', () => {
  let server: ReturnType<typeof setupServer> | null;

  afterEach(() => {
    if (server) {
      server.close();
    }
    server = null;
  });

  describe('shareWithFlamegraphDotcom', () => {
    it('works', async () => {
      server = setupServer(
        rest.post(`http://localhost/export`, (req, res, ctx) => {
          return res(ctx.status(200), ctx.json({ url: 'http://myurl.com' }));
        })
      );

      server.listen();
      const res = await shareWithFlamegraphDotcom({
        name: 'myname',
        flamebearer: TestData,
      });

      expect(res).toMatchObject(
        Result.ok({
          url: 'http://myurl.com',
        })
      );
    });

    it('fails if response doesnt contain the key', async () => {
      server = setupServer(
        rest.post(`http://localhost/export`, (req, res, ctx) => {
          return res(ctx.status(200), ctx.json({}));
        })
      );
      server.listen();

      const res = await shareWithFlamegraphDotcom({
        name: 'myname',
        flamebearer: TestData,
      });
      expect(res.error).toBeInstanceOf(ZodError);
    });
  });
});
