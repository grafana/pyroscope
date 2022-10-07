/* eslint-disable */
import { PlotType } from './types';

// function taken from markings support in Flot
export default function extractRange(
  plot: PlotType,
  ranges: { [x: string]: any },
  coord: string
) {
  var axis,
    from,
    to,
    key,
    axes = plot.getAxes();

  for (var k in axes) {
    axis = axes[k];
    if (axis.direction == coord) {
      key = coord + axis.n + 'axis';
      if (!ranges[key] && axis.n == 1) key = coord + 'axis'; // support x1axis as xaxis
      if (ranges[key]) {
        from = ranges[key].from;
        to = ranges[key].to;
        break;
      }
    }
  }

  // backwards-compat stuff - to be removed in future
  if (!ranges[key as string]) {
    axis = coord == 'x' ? plot.getXAxes()[0] : plot.getYAxes()[0];
    from = ranges[coord + '1'];
    to = ranges[coord + '2'];
  }

  // auto-reverse as an added bonus
  if (from != null && to != null && from > to) {
    var tmp = from;
    from = to;
    to = tmp;
  }

  return { from: from, to: to, axis: axis };
}
