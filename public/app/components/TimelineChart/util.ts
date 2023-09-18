export type TimelineChartSelectionTimeRangeAxis = {
  from: number;
  to: number;
};

export type TimelineAxisSelection = {
  from: number;
  to: number;
};

export type TimelineVisibleData = Array<{
  data: number[][];
}>;

/**
 * Evaluates if a time range contains points suitable to zoom into.
 * The criteria is that there should be at least two points on the same data set.
 * If the number of xaxis pixels is smaller than number of points in a dataset,
 * we assume that no xaxisRange can be selected that won't contain at least two points.
 *
 * @param xaxisRange mouse drag/drop selection range along the xaxis
 * @param datasets a collection of datasets that are currently visible on the chart
 * @param xaxisPixels the width of the chart in pixels
 * @returns
 */
export function rangeIsAcceptableForZoom(
  xaxisRange: TimelineAxisSelection,
  datasets: TimelineVisibleData,
  xaxisPixels: number
) {
  if (xaxisRange == null) {
    // Invalid range, do nothing.
    return false;
  }

  const { from, to } = xaxisRange;

  for (const dataset of datasets) {
    const points = dataset.data;

    if (points.length > xaxisPixels) {
      // There are more points than pixels visible,
      // so we can assume the range is safe.
      return true;
    }

    let pointCount = 0;
    for (const point of points) {
      // If at least two points are on the range, it is acceptable.
      if (point[0] > from && point[0] < to) {
        pointCount++;
        if (pointCount >= 2) {
          return true;
        }
      }
    }
  }

  return false;
}
