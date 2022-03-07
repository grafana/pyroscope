import { MetadataSchema } from './profile';

describe('Profile', () => {
  describe('MetadataSchem', () => {
    describe('Units', () => {
      const expected = {
        format: 'single',
        sampleRate: 100,
        spyName: '',
        units: '',
      };

      it('accepts empty units', () => {
        const metadata = {
          format: 'single',
          sampleRate: 100,
          spyName: '',
          units: '',
        };
        expect(MetadataSchema.parse(metadata)).toStrictEqual(expected);
      });

      it("accepts 'undefined' units", () => {
        const metadata = {
          format: 'single',
          sampleRate: 100,
          spyName: '',
        };

        expect(MetadataSchema.parse(metadata)).toStrictEqual(expected);
      });

      it("defaults unidentified units to ''", () => {
        const metadata = {
          format: 'single',
          sampleRate: 100,
          spyName: '',
          units: 'my_custom_unit',
        };

        expect(MetadataSchema.parse(metadata)).toStrictEqual(expected);
      });
    });
  });
});
