type SeriesSize = 'sm' | 'md' | 'lg';

export interface SimpleOptions {
  text: string;
  showSeriesCount: boolean;
  seriesCountSize: SeriesSize;

  showToolbar: boolean;
  displayOnly: 'flamegraph' | 'table' | 'both' | 'sandwich' | 'off';
}
