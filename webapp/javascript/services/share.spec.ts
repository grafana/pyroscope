import { Result } from '@utils/fp';
import { shareWithFlamegraphDotcom } from './share';
import { setupServer, rest } from './testUtils';
// TODO move this testData to somewhere else
import TestData from '../components/FlameGraph/FlameGraphComponent/testData';

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
        rest.get(`http://localhost/export`, (req, res, ctx) => {
          // TODO check query params
          //
          return res(
            ctx.status(200),

            ctx.json({
              url: 'http://myurl.com',
            })
          );
        })
      );

      server.listen();
      const res = await shareWithFlamegraphDotcom({
        name: 'myname',
        flamebearer: TestData.SimpleTree,
      });

      expect(res).toMatchObject(
        Result.ok({
          url: 'http://myurl.com',
        })
      );
    });
  });
});
