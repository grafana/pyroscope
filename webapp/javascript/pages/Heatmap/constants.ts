import Color from 'color';

export const HEATMAP_HEIGHT = 250;

// TODO(dogfrogfog): use colors in the same format
export const SELECTED_AREA_BACKGROUND = Color.rgb(255, 255, 0)
  .alpha(0.5)
  .toString();

export const COLOR_EMPTY = [22, 22, 22];
export const COLOR_2 = [202, 240, 248];
export const COLOR_1 = [3, 4, 94];

export const DEFAULT_HEATMAP_PARAMS = {
  minValue: 0,
  maxValue: 1000000000,
  heatmapTimeBuckets: 128,
  heatmapValueBuckets: 32,
};
