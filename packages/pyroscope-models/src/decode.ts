import { Profile } from './profile';

// TODO: ideally this should be moved into the FlamegraphRenderer component
// but since it will require too many changes for now
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

// eslint-disable-next-line import/prefer-default-export
export function decodeFlamebearer(fb: Profile): Profile {
  // TODO: make this immutable
  // eslint-disable-next-line no-param-reassign
  fb.flamebearer.levels = deltaDiffWrapper(
    fb.metadata.format,
    fb.flamebearer.levels
  );
  return fb;
}
