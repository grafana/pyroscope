import { Result } from '@utils/fp';
import { get } from './base';
import { setupServer, rest } from './testUtils';

const baseURL = 'http://localhost';
const server = setupServer(
  rest.get(`${baseURL}/test`, (req, res, ctx) => {
    return res(
      ctx.status(500),
      ctx.json({ message: 'Deliberately broken request' })
    );
  })
);

describe('Base HTTP', () => {
  beforeAll(() => {
    (window as any).baseURL = baseURL;
    server.listen();
  });
  afterAll(() => {
    server.close();
  });

  it('works', async () => {
    const res = await get<any>('/test');

    expect(res).toMatchObject(
      Result.err({
        statusCode: 500,
        message: 'Request failed',
      })
    );
  });
});
