import { MetadataSchema, FlamebearerSchema } from './profile';

describe('Profile', () => {
  describe('FlamebearerSchema', () => {
    describe('Names', () => {
      it('defaults to "unknown" when name is empty', () => {
        const fb = {
          names: ['', ''],
          levels: [],
          numTicks: 100,
          maxSelf: 100,
        };
        expect(FlamebearerSchema.parse(fb)).toStrictEqual({
          names: ['unknown', 'unknown'],
          levels: [],
          numTicks: 100,
          maxSelf: 100,
        });
      });

      it('uses correct name when not empty', () => {
        const fb = {
          names: ['myname', 'myname2'],
          levels: [],
          numTicks: 100,
          maxSelf: 100,
        };
        expect(FlamebearerSchema.parse(fb)).toStrictEqual(fb);
      });
    });
  });

  describe('MetadataSchem', () => {
    describe('Units', () => {
      const expected = {
        format: 'single',
        sampleRate: 100,
        spyName: 'unknown',
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
