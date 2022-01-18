import { Units } from '@utils/format';
import { deltaDiffWrapper } from '@utils/flamebearer';

export type Flamebearer = {
  /**
   * List of names
   */
  names: string[];
  /**
   * List of level
   *
   * This is NOT the same as in the flamebearer
   * that we receive from the server.
   * As in there are some transformations required
   * (see deltaDiffWrapper)
   */
  levels: number[][];
  numTicks: number;
  /**
   * Sample Rate, used in text information
   */
  sampleRate: number;
  units: Units;
  spyName:
    | 'dotneyspy'
    | 'ebpfspy'
    | 'gospy'
    | 'phpspy'
    | 'pyspy'
    | 'rbspy'
    | string;
  /**
   * Format version.
   */
  version: number;
} & addTicks;

export type addTicks =
  | { format: 'double'; leftTicks: number; rightTicks: number }
  | { format: 'single' };

interface DecodeFlamebearerProps {
  flamebearer: Flamebearer;
  metadata: {
    format: string;
    spyName: string;
    sampleRate: number;
    units: Units;
  };
  leftTicks?: number;
  rightTicks?: number;
  version?: number;
}

// Hopefully these type assertions won't be required once we enable strictNullChecks in the ompiler
export function decodeFlamebearer({
  flamebearer,
  metadata,
  leftTicks,
  rightTicks,
  version,
}: DecodeFlamebearerProps): Flamebearer {
  const fb = {
    ...flamebearer,
    format: metadata.format,
    spyName: metadata.spyName,
    sampleRate: metadata.sampleRate,
    units: metadata.units,
  };

  if (fb.format === 'double') {
    (fb as any).leftTicks = leftTicks;
    (fb as any).rightTicks = rightTicks;
  }

  fb.version = version || 0;
  fb.levels = deltaDiffWrapper(fb.format, fb.levels);
  return fb as Flamebearer;
}

export type FlamebearerProfile = {
  Flamebearer: Flamebearer;

  metadata: {
    appName: string;
    startTime: string;
    endTime: string;
    query: string;
    maxNodes: number;
  };
};
