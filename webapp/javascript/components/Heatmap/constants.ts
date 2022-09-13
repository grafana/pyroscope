import Color from 'color';

export const HEATMAP_HEIGHT = 250;

export const SELECTED_AREA_BACKGROUND = Color.rgb(240, 240, 240)
  .alpha(0.5)
  .toString();

export const DEFAULT_HEATMAP_PARAMS = {
  minValue: 0,
  maxValue: 1000000000,
  heatmapTimeBuckets: 128,
  heatmapValueBuckets: 32,
};

export const VIRIDIS_COLORS = [
  Color.rgb(253, 231, 37),
  Color.rgb(94, 201, 98),
  Color.rgb(33, 145, 140),
  Color.rgb(59, 82, 139),
  Color.rgb(68, 1, 84),
];
