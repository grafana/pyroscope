import { Units } from '@utils/format';

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
