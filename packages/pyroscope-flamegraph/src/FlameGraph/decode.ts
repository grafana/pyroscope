import { Profile } from '@pyroscope/models';

function deltaDiffWrapper(
  format: Profile['metadata']['format'],
  levels: Profile['flamebearer']['levels']
) {
  const mutableLevels = [...levels];

  function deltaDiff(
    lvls: Profile['flamebearer']['levels'],
    start: number,
    step: number
  ) {
    // eslint-disable-next-line no-restricted-syntax
    for (const level of lvls) {
      let prev = 0;
      for (let i = start; i < level.length; i += step) {
        level[i] += prev;
        prev = level[i] + level[i + 1];
      }
    }
  }

  if (format === 'double') {
    deltaDiff(mutableLevels, 0, 7);
    deltaDiff(mutableLevels, 3, 7);
  } else {
    deltaDiff(mutableLevels, 0, 4);
  }

  return mutableLevels;
}

export default function decodeFlamebearer(fb: Profile): Profile {
  // Make a copy since we will modify the undelying data structure
  const copy = JSON.parse(JSON.stringify(fb));

  copy.flamebearer.levels = deltaDiffWrapper(
    copy.metadata.format,
    copy.flamebearer.levels
  );

  return copy;
}
