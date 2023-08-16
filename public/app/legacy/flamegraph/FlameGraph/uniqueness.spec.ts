import fs from 'fs';
import path from 'path';
import { isSameFlamebearer } from './uniqueness';
import { normalize } from './normalize';

// Just to make it easier to import "heavy" data
const testData = JSON.parse(
  fs.readFileSync(path.join(__dirname, './testData.json'), 'utf-8')
);
const flame = normalize({ profile: testData });

describe('uniqueness', () => {
  it('works', () => {
    expect(isSameFlamebearer(flame, flame)).toBe(true);
  });
});
