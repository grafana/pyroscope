import { Flamebearer } from '@pyroscope/legacy/models';

export function isSameFlamebearer(
  prevFlame: Flamebearer,
  currFlame: Flamebearer
) {
  // We first compare simple fields, since they are faster
  if (prevFlame.format !== currFlame.format) {
    return false;
  }

  if (prevFlame.numTicks !== currFlame.numTicks) {
    return false;
  }

  if (prevFlame.sampleRate !== currFlame.sampleRate) {
    return false;
  }

  if (prevFlame.units !== currFlame.units) {
    return false;
  }

  if (prevFlame.names?.length !== currFlame?.names.length) {
    return false;
  }

  if (prevFlame.levels.length !== currFlame.levels.length) {
    return false;
  }

  // Most likely names is smaller, so let's start with it
  // Are all names the same?
  if (
    !prevFlame.names.every((a, i) => {
      return a === currFlame.names[i];
    })
  ) {
    return false;
  }

  if (!areLevelsTheSame(prevFlame.levels, currFlame.levels)) {
    return false;
  }

  // Fallback in case new fields are added
  return (
    JSON.stringify({
      ...prevFlame,
      levels: undefined,
      names: undefined,
    }) ===
    JSON.stringify({
      ...currFlame,
      levels: undefined,
      names: undefined,
    })
  );
}

function areLevelsTheSame(
  l1: Flamebearer['levels'],
  l2: Flamebearer['levels']
) {
  if (l1.length !== l2.length) {
    return false;
  }

  // eslint-disable-next-line no-plusplus
  for (let i = 0; i < l1.length; i++) {
    if (l1[i].length !== l2[i].length) {
      return false;
    }

    // eslint-disable-next-line no-plusplus
    for (let j = 0; j < l1[i].length; j++) {
      if (l1[i][j] !== l2[i][j]) {
        return false;
      }
    }
  }

  return true;
}
