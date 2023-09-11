import { formatAsOBject } from '@pyroscope/util/formatDate';
import Color from 'color';

// Same green as button
export const ANNOTATION_COLOR = Color('#2ecc40');

type FlotMarkings = Array<{
  xaxis: {
    from: number;
    to: number;
  };
  yaxis?: {
    from: number;
    to: number;
  };
  color: Color;
}>;

// Unify these types
export interface Selection {
  from: string;
  to: string;
  color: Color;
  overlayColor: Color;
}

// FIXME: right now the selection functionality is spread in 2 places
// normal markings (function below)
// and in the TimelineChartSelection plugin
// which is confusing and should be fixed
function constructSelection(
  m: Selection,
  selectionType: 'double' | 'single'
): FlotMarkings {
  const from = new Date(formatAsOBject(m.from)).getTime();
  const to = new Date(formatAsOBject(m.to)).getTime();

  // 'double' selection uses built-in Flot selection
  // built-in Flot selection for 'single' case becomes 'transparent'
  // to use custom apperance and color for it
  const boundary = {
    lineWidth: 1,
    color: selectionType === 'double' ? m.color.rgb() : Color('transparent'),
  };

  return [
    {
      xaxis: { from, to },
      color: selectionType === 'double' ? m.overlayColor : Color('transparent'),
    },

    // A single vertical line indicating boundaries from both sides (left and right)
    { ...boundary, xaxis: { from, to: from } },
    { ...boundary, xaxis: { from: to, to } },
  ];
}

/**
 * generate markings from selection
 */
export function markingsFromSelection(
  selectionType: 'single' | 'double',
  left?: Selection,
  right?: Selection
): FlotMarkings {
  let res: FlotMarkings = [];

  if (left) {
    res = res.concat(constructSelection(left, selectionType));
  }
  if (right) {
    res = res.concat(constructSelection(right, selectionType));
  }
  return res;
}
