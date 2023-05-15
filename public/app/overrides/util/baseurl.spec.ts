import baseurlForAPI, { baseurl } from '@webapp/util/baseurl';

function mockSelector(href: string) {
  const base = document.createElement('base');
  base.href = href;
  return base;
}

describe('baseurlForAPI', () => {
  describe('no baseurl has been detected', () => {
    it('returns undefined', () => {
      const got = baseurlForAPI();
      expect(got).toBe(undefined);
    });
  });

  describe('base tag is set', () => {
    describe('it contains /ui', () => {
      it('removes /ui path', () => {
        jest
          .spyOn(document, 'querySelector')
          .mockImplementationOnce(() => mockSelector('/pyroscope/ui'));

        const got = baseurlForAPI();
        expect(got).toBe('/pyroscope');
      });
    });
  });
});

describe('baseurl', () => {
  describe('no baseURL meta tag set', () => {
    it('returns undefined', () => {
      const got = baseurl();
      expect(got).toBe(undefined);
    });
  });

  describe('base tag is set', () => {
    describe('a relative path is passed', () => {
      it('uses as is', () => {
        jest
          .spyOn(document, 'querySelector')
          .mockImplementationOnce(() => mockSelector('/pyroscope'));

        const got = baseurl();
        expect(got).toBe('/pyroscope');
      });
    });

    describe('a full url is passed', () => {
      it('strips the origin', () => {
        jest
          .spyOn(document, 'querySelector')
          .mockImplementationOnce(() => mockSelector('/pyroscope'));

        const got = baseurl();
        expect(got).toBe('/pyroscope');
      });
    });
  });
});
