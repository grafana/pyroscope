/**
 * @deprecated one should use the Profile model
 */
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
  units: 'samples' | 'objects' | 'bytes' | '';
  spyName:
    | 'dotneyspy'
    | 'ebpfspy'
    | 'gospy'
    | 'phpspy'
    | 'pyspy'
    | 'rbspy'
    | string;
} & addTicks;

export type addTicks =
  | { format: 'double'; leftTicks: number; rightTicks: number }
  | { format: 'single' };
