import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import SyncTimelines from './index';
import { getTitle } from './useSync';
import { getSelectionBoundaries } from './getSelectionBoundaries';
import { Selection } from '../markings';

const from = 1666790156;
const to = 1666791905;

const propsWhenActive = {
  timeline: {
    color: 'rgb(208, 102, 212)',
    data: {
      startTime: 1666790760,
      samples: [
        16629, 50854, 14454, 3819, 40720, 23172, 22483, 7854, 33186, 81804,
        46942, 40631, 14135, 12824, 27514, 14366, 39691, 45412, 18631, 10371,
        31606, 53775, 42399, 40527, 20599, 27836, 23624, 80152, 9149, 45283,
        58361, 48738, 30363, 13834, 30849, 81892,
      ],
      durationDelta: 10,
    },
  },
  leftSelection: {
    from: String(from),
    to: '1666790783',
  },
  rightSelection: {
    from: '1666791459',
    to: String(to),
  },
  comparisonModeActive: false,
};

const propsWhenHidden = {
  timeline: {
    data: {
      startTime: 1666779070,
      samples: [
        1601, 30312, 22044, 53925, 44264, 26014, 15645, 14376, 21880, 8555,
        15995, 5849, 14138, 18929, 41842, 59101, 18931, 65541, 47674, 35886,
        55583, 19283, 19745, 9314, 1531,
      ],
      durationDelta: 10,
    },
  },
  leftSelection: {
    from: '1666779093',
    to: '1666779239',
  },
  rightSelection: {
    from: '1666779140',
    to: '1666779296',
  },
  comparisonModeActive: false,
};

const { getByRole, queryByText } = screen;

describe('SyncTimelines', () => {
  it('renders sync and ignore buttons when active', async () => {
    render(<SyncTimelines onSync={() => {}} {...propsWhenActive} />);

    expect(getByRole('button', { name: 'Ignore' })).toBeInTheDocument();
    expect(getByRole('button', { name: 'Sync Timelines' })).toBeInTheDocument();
  });

  it('hidden when selections are in range', async () => {
    render(<SyncTimelines onSync={() => {}} {...propsWhenHidden} />);

    expect(queryByText('Sync')).not.toBeInTheDocument();
  });

  it('onSync returns correct from/to', async () => {
    let result = { from: '', to: '' };
    render(
      <SyncTimelines
        {...propsWhenActive}
        onSync={(from, to) => {
          result = { from, to };
        }}
      />
    );

    fireEvent.click(getByRole('button', { name: 'Sync Timelines' }));

    // new main timeline FROM = from - 1ms, TO = to + 1ms
    expect(Number(result.from) - from * 1000).toEqual(-1);
    expect(Number(result.to) - to * 1000).toEqual(1);
  });

  it('Hide button works', async () => {
    render(<SyncTimelines onSync={() => {}} {...propsWhenActive} />);

    fireEvent.click(getByRole('button', { name: 'Ignore' }));

    expect(queryByText('Sync')).not.toBeInTheDocument();
  });
});

describe('getTitle', () => {
  it('both selections are out of range', () => {
    expect(getTitle(false, false)).toEqual(
      'Warning: Baseline and Comparison timeline selections are out of range'
    );
  });
  it('baseline timeline selection is out of range', () => {
    expect(getTitle(false, true)).toEqual(
      'Warning: Baseline timeline selection is out of range'
    );
  });
  it('comparison timeline selection is out of range', () => {
    expect(getTitle(true, false)).toEqual(
      'Warning: Comparison timeline selection is out of range'
    );
  });
});

describe('getSelectionBoundaries', () => {
  const boundariesFromRelativeTime = getSelectionBoundaries({
    from: 'now-1h',
    to: 'now',
  } as Selection);
  const boundariesFromUnixTime = getSelectionBoundaries({
    from: '1667204605',
    to: '1667204867',
  } as Selection);

  const res = [
    boundariesFromRelativeTime.from,
    boundariesFromRelativeTime.to,
    boundariesFromUnixTime.from,
    boundariesFromUnixTime.to,
  ];

  it('returns correct data type', () => {
    expect(res.every((i) => typeof i === 'number')).toBe(true);
  });

  it('returns ms format (13 digits)', () => {
    expect(res.every((i) => String(i).length === 13)).toBe(true);
  });

  it('TO greater than FROM', () => {
    expect(
      boundariesFromRelativeTime.to > boundariesFromRelativeTime.from &&
        boundariesFromUnixTime.to > boundariesFromUnixTime.from
    ).toBe(true);
  });
});
