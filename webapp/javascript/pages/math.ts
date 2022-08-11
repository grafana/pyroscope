export function calculateMean(arr: number[]) {
  return arr.reduce((acc, b) => acc + b, 0) / arr.length;
}

export function calculateStdDeviation(array: number[], mean: number) {
  const stdDeviation = Math.sqrt(
    array.reduce((acc, b) => {
      const dev = b - mean;

      return acc + dev ** 2;
    }, 0) / array.length
  );

  return stdDeviation;
}
