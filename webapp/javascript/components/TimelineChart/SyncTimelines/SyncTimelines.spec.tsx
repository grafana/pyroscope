import React from 'react';
import { render, screen } from '@testing-library/react';
import SyncTimelines from './index';
import { getTitle } from './useSync';

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
};

const propsWhenActive = {
  timeline: {
    data: {
      startTime: 1666779010,
      samples: [
        53137, 55457, 5257, 52082, 56993, 21293, 16066, 30312, 22044, 53925,
        44264, 26014, 15645,
      ],
      durationDelta: 10,
    },
  },
  leftSelection: {
    from: 'now-1h',
    to: 'now-30m',
  },
  rightSelection: {
    from: 'now-30m',
    to: 'now',
  },
};

describe('SyncTimelines', () => {
  it('renders sync and ignore buttons when active', async () => {
    render(<SyncTimelines onSync={() => {}} {...propsWhenActive} />);

    expect(screen.getByTestId('sync-ignore-button')).toBeInTheDocument();
    expect(screen.getByTestId('sync-button')).toBeInTheDocument();
  });

  it('hidden when selections are in range', async () => {
    render(<SyncTimelines onSync={() => {}} {...propsWhenHidden} />);

    expect(screen.queryByText('Sync')).not.toBeInTheDocument();
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
