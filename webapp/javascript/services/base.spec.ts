import { Result } from '@utils/fp';
import { get } from './base';
import { setupServer, rest } from './testUtils';

describe('Base HTTP', () => {
  let server: ReturnType<typeof setupServer>;

  afterEach(() => {
    if (server) {
      server.close();
    }
    server = null;
  });

  describe('Server responds', () => {
    it('with valid JSON data', async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(
            ctx.status(200),
            ctx.json({
              foo: 'bar',
            })
          );
        })
      );
      server.listen();
      const res = await get('/test');

      expect(res).toMatchObject(
        Result.ok({
          foo: 'bar',
        })
      );
    });

    it('with invalid JSON data', async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(ctx.status(200), ctx.text('bla'));
        })
      );
      server.listen();
      const res = await get('/test');

      expect(res).toMatchObject(
        Result.err({
          message: 'Failed to parse JSON',
          data: 'bla',
        })
      );
    });
  });

  describe('Server never responded', () => {
    it('fails', async () => {
      const res = await get('/test');

      expect(res).toMatchObject(
        Result.err({
          message:
            'request to http://localhost/test failed, reason: connect ECONNREFUSED 127.0.0.1:80',
        })
      );
    });
  });

  describe('Server responded with 2xx and data', () => {
    it('works', () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(ctx.status(200), ctx.json({}));
        })
      );
      server.listen();
    });
  });

  describe('Server responded with statusCode outside 2xx range', () => {
    it(`Returns a default message if theres no body`, async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(ctx.status(500));
        })
      );
      server.listen();

      const res = await get('/test');

      expect(res).toMatchObject(
        Result.err({
          statusCode: 500,
          message: 'Request failed',
        })
      );
    });

    it(`Returns the body when there's a body in JSON format`, async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(ctx.status(500), ctx.json({ message: 'server error' }));
        })
      );
      server.listen();

      const res = await get('/test');

      expect(res).toMatchObject(
        Result.err({
          statusCode: 500,
          message: 'server error',
        })
      );
    });

    it(`Returns the text body as message when there's response NOT in JSON format`, async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(ctx.status(500), ctx.text('text error'));
        })
      );
      server.listen();

      const res = await get('/test');

      expect(res).toMatchObject(
        Result.err({
          statusCode: 500,
          message: 'text error',
        })
      );
    });
  });
});
