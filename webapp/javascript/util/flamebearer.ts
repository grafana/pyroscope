function deltaDiffWrapper(format: 'single' | 'double', levels: number[][]) {
  const mutable_levels = [...levels];

  function deltaDiff(levels: number[][], start: number, step: number) {
    for (const level of levels) {
      let prev = 0;
      for (let i = start; i < level.length; i += step) {
        level[i] += prev;
        prev = level[i] + level[i + 1];
      }
    }
  }

  if (format === 'double') {
    deltaDiff(mutable_levels, 0, 7);
    deltaDiff(mutable_levels, 3, 7);
  } else {
    deltaDiff(mutable_levels, 0, 4);
  }

  return mutable_levels;
}

export { deltaDiffWrapper };
