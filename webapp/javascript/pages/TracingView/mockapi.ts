export const startTime = 1451433600000; // x start (unix)
export const endTime = 1454946120000; // x end (unix)
export const minDepth = 20; // min heatmap value non-zero (for color) should be lighest not white color
export const maxDepth = 226; // max heatmap value non-zero (for color) should be the darkest color
export const valueBuckets = 30; // columns number (y axis)
export const timeBuckets = 160; // rows number (x axis)
export const columns = Array(valueBuckets)
  .fill(Array(timeBuckets).fill(1))
  .map((col) =>
    col.map((_: number) => {
      const rand = Math.floor(Math.random() * maxDepth + minDepth);

      // to add more 0 (white squares)
      if (rand > 50 && rand < 210) {
        return 0;
      }

      return rand;
    })
  );
