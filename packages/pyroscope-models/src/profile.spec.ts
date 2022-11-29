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
});
