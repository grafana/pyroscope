import Color from 'color';

export const HEATMAP_HEIGHT = 250;

export const SELECTED_AREA_BACKGROUND = Color.rgb(240, 240, 240).alpha(0.5);

export const DEFAULT_HEATMAP_PARAMS = {
  minValue: 0,
  maxValue: 1000000000,
  heatmapTimeBuckets: 128,
  heatmapValueBuckets: 32,
};

// viridis palette
export const HEATMAP_COLORS = [
  Color.rgb(253, 231, 37),
  Color.rgb(174, 216, 68),
  Color.rgb(94, 201, 98),
  Color.rgb(33, 145, 140),
  Color.rgb(59, 82, 139),
  Color.rgb(64, 42, 112),
  Color.rgb(68, 1, 84),
];
