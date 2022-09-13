import Color from 'color';
import {
  ANNOTATION_COLOR,
  ANNOTATION_WIDTH,
  markingsFromAnnotations,
  markingsFromSelection,
} from './markings';

describe('markingsFromAnnotations', () => {
  it('works', () => {
    const timestamp = 1663000000;
    const annotations = [
      {
        timestamp,
      },
    ];
    expect(markingsFromAnnotations(annotations)).toStrictEqual([
      {
        lineWidth: ANNOTATION_WIDTH,
        color: ANNOTATION_COLOR,
        xaxis: {
          from: timestamp * 1000,
          to: timestamp * 1000,
        },
      },
    ]);
  });
});

// Tests are definitely confusing, but that's due to the nature of the implementation
// TODO: refactor implementatino
describe('markingsFromSelection', () => {
  it('returns nothing when theres no selection', () => {
    expect(markingsFromSelection('single')).toStrictEqual([]);
  });

  const from = 1663000000;
  const to = 1665000000;
  const color = Color('red');

  it('ignores color when selection is single', () => {
    expect(
      markingsFromSelection('single', {
        from: `${from}`,
        to: `${to}`,
        color,
        overlayColor: color,
      })
    ).toStrictEqual([
      {
        color: Color('transparent'),
        xaxis: {
          from: from * 1000,
          to: to * 1000,
        },
      },
      {
        color: Color('transparent'),
        lineWidth: 1,
        xaxis: {
          from: from * 1000,
          to: from * 1000,
        },
      },
      {
        color: Color('transparent'),
        lineWidth: 1,
        xaxis: {
          from: to * 1000,
          to: to * 1000,
        },
      },
    ]);
  });

  it('uses color when selection is double', () => {
    expect(
      markingsFromSelection('double', {
        from: `${from}`,
        to: `${to}`,
        color: color,
        overlayColor: color,
      })
    ).toStrictEqual([
      {
        color: color,
        xaxis: {
          from: from * 1000,
          to: to * 1000,
        },
      },
      {
        color: color,
        lineWidth: 1,
        xaxis: {
          from: from * 1000,
          to: from * 1000,
        },
      },
      {
        color: color,
        lineWidth: 1,
        xaxis: {
          from: to * 1000,
          to: to * 1000,
        },
      },
    ]);
  });
});
