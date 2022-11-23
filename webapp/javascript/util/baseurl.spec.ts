import basename from './baseurl';

function checkSelector(selector: string) {
  if (selector !== 'meta[name="pyroscope-base-url"]') {
    throw new Error('Wrong selector');
  }
}
describe('baseurl', () => {
  describe('no baseURL meta tag set', () => {
    it('returns undefined', () => {
      const got = basename();

      expect(got).toBe(undefined);
    });
  });

  describe('baseURL meta tag set', () => {
    describe('no content', () => {
      beforeEach(() => {
        jest
          .spyOn(document, 'querySelector')
          .mockImplementationOnce((selector) => {
            checkSelector(selector);
            return {} as HTMLMetaElement;
          });
      });
      it('returns undefined', () => {
        const got = basename();

        expect(got).toBe(undefined);
      });
    });

    describe("there's content", () => {
      it('works with a base Path', () => {
        jest
          .spyOn(document, 'querySelector')
          .mockImplementationOnce((selector) => {
            checkSelector(selector);
            return { content: '/pyroscope' } as HTMLMetaElement;
          });

        const got = basename();

        expect(got).toBe('/pyroscope');
      });

      it('works with a full URL', () => {
        jest
          .spyOn(document, 'querySelector')
          .mockImplementationOnce((selector) => {
            checkSelector(selector);
            return {
              content: 'http://localhost:8080/pyroscope',
            } as HTMLMetaElement;
          });

        const got = basename();

        expect(got).toBe('/pyroscope');
      });
    });
  });
});
