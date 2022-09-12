import { formatAsOBject } from '@webapp/util/formatDate';
import Color from 'color';

type FlotMarkings = {
  xaxis: {
    from: number;
    to: number;
  };
  yaxis?: {
    from: number;
    to: number;
  };
  color: Color;
}[];

/**
 * generate markings in flotjs format
 */
export function markingsFromAnnotations(
  annotations?: { timestamp: number }[]
): FlotMarkings {
  // Same green as button
  const ANNOTATION_COLOR = Color('#2ecc40');
  const ANNOTATION_WIDTH = '2px';

  if (!annotations?.length) {
    return [];
  }

  return annotations.map((a) => ({
    xaxis: {
      // TODO(eh-am): look this up
      from: a.timestamp * 1000,
      to: a.timestamp * 1000,
    },
    lineWidth: ANNOTATION_WIDTH,
    color: ANNOTATION_COLOR,
  }));
}

// Unify these types
interface Selection {
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
      color: selectionType === 'double' ? m.overlayColor : Color('NOOP'),
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
  const res: FlotMarkings = [];

  if (left) {
    res.concat(constructSelection(left, selectionType));
  }
  if (right) {
    res.concat(constructSelection(right, selectionType));
  }

  return res;
}
