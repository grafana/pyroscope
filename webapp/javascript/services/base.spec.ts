import { Result } from '@webapp/util/fp';
import path from 'path';
import { ZodError } from 'zod';
import {
  request,
  mountRequest,
  RequestNotOkError,
  RequestNotOkWithErrorsList,
  ResponseOkNotInJSONFormat,
  RequestIncompleteError,
  ResponseNotOkInHTMLFormat,
} from './base';
import { setupServer, rest } from './testUtils';
import basename from '../util/baseurl';

jest.mock('../util/baseurl', () => jest.fn());

describe('Base HTTP', () => {
  let server: ReturnType<typeof setupServer> | null;

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
      const res = await request('/test');

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
      const res = await request('/test');

      expect(res.error).toBeInstanceOf(ResponseOkNotInJSONFormat);
      expect(res.error.message).toBe(
        "Server returned with code: '200'. The body that could not be parsed contains 'bla'"
      );
    });
  });

  describe('Server never responded', () => {
    it('fails', async () => {
      const res = await request('/test');

      expect(res.error).toBeInstanceOf(RequestIncompleteError);
      expect(res.error.message).toBe(
        "Request failed to be completed. Description: 'request to http://localhost/test failed, reason: connect ECONNREFUSED 127.0.0.1:80'"
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

      const res = await request('/test');

      expect(res.error).toBeInstanceOf(RequestNotOkError);
      expect(res.error.message).toBe(
        "Request failed with statusCode: '500' and description: 'No description available'"
      );
    });

    it(`Returns an error list if available`, async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(
            ctx.status(500),
            ctx.json({
              errors: ['error1', 'error2'],
            })
          );
        })
      );
      server.listen();

      const res = await request('/test');

      expect(res.error).toBeInstanceOf(RequestNotOkWithErrorsList);
      expect(res.error.message).toBe('Error(s) were found: "error1", "error2"');
    });

    it(`Returns an error message if available`, async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(
            ctx.status(500),
            ctx.json({
              error: 'error',
            })
          );
        })
      );
      server.listen();

      const res = await request('/test');

      expect(res.error).toBeInstanceOf(RequestNotOkError);
      expect(res.error.message).toBe(
        "Request failed with statusCode: '500' and description: 'error'"
      );
    });

    it(`Returns an error message if available`, async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(
            ctx.status(500),
            ctx.json({
              message: 'error',
            })
          );
        })
      );
      server.listen();

      const res = await request('/test');

      expect(res.error).toBeInstanceOf(RequestNotOkError);
      expect(res.error.message).toBe(
        "Request failed with statusCode: '500' and description: 'error'"
      );
    });

    it(`Returns a bunch of data`, async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(
            ctx.status(500),
            ctx.json({
              foo: 'foo',
              bar: 'bar',
            })
          );
        })
      );
      server.listen();

      const res = await request('/test');

      expect(res.error).toBeInstanceOf(RequestNotOkError);
      expect(res.error.message).toBe(
        // eslint-disable-next-line no-useless-escape
        `Request failed with statusCode: '500' and description: 'Could not identify an error message. Payload is {\"foo\":\"foo\",\"bar\":\"bar\"}'`
      );
    });

    it(`Returns the text body as message when there's response NOT in JSON format`, async () => {
      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(ctx.status(500), ctx.text('text error'));
        })
      );
      server.listen();

      const res = await request('/test');

      expect(res.error).toBeInstanceOf(RequestNotOkError);
      expect(res.error.message).toBe(
        "Request failed with statusCode: '500' and description: 'text error'"
      );
    });

    it('Returns a generic message when respond with HTML data', async () => {
      const htmlData = require('fs').readFileSync(
        path.resolve(__dirname, './testdata/example.html'),
        'utf8'
      );

      server = setupServer(
        rest.get(`http://localhost/test`, (req, res, ctx) => {
          return res(ctx.status(500), ctx.text(htmlData));
        })
      );
      server.listen();
      const res = await request('/test');

      expect(res.error).toBeInstanceOf(ResponseNotOkInHTMLFormat);
      expect(res.error.message).toBe(
        "Server returned with code: '500'. The body contains an HTML page"
      );
    });
  });
});

// Normally this wouldn't be tested
// But since implementation is complex enough
// It's better to expose and test it
// TODO test when req is an object
describe('mountRequest', () => {
  describe('basename is set', () => {
    it('prepends browserURL with basename', () => {
      (basename as any).mockImplementationOnce(() => {
        return '/pyroscope';
      });

      const got = mountRequest('my-request');
      expect(got).toBe('http://localhost/pyroscope/my-request');
    });
  });

  describe('basename is NOT set', () => {
    it('returns the browser url', () => {
      (basename as any).mockImplementationOnce(() => {
        return null;
      });

      const got = mountRequest('my-request');
      expect(got).toBe('http://localhost/my-request');
    });
  });
});
