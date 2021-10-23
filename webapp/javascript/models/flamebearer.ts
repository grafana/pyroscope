import { Units } from '@utils/format';

type addTicks =
  | { format: 'double'; leftTicks: number; rightTicks: number }
  | { format: 'single' };

export type Flamebearer = {
  names: string[];
  levels: number[][];
  numTicks: number;
  sampleRate: number;
  units: Units;
  spyName: string;
} & addTicks;
