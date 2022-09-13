import { fetchSelectionProfile } from '@webapp/redux/reducers/tracing';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import type { Heatmap } from '@webapp/services/render';
import { DEFAULT_HEATMAP_PARAMS, HEATMAP_HEIGHT } from './constants';

interface UseHeatmapProfile {
  fetchProfile: (
    xS: number,
    xE: number,
    yS: number,
    yE: number,
    edgeClick?: boolean
  ) => void;
}

interface UseHeatmapProfileProps {
  heatmapData?: Heatmap;
  heatmapW: number;
}

export const useHeatmapProfile = ({
  heatmapData,
  heatmapW,
}: UseHeatmapProfileProps): UseHeatmapProfile => {
  const dispatch = useAppDispatch();
  const { from, until, query } = useAppSelector((state) => state.continuous);

  const fetchProfile = (
    xStart: number,
    xEnd: number,
    yStart: number,
    yEnd: number,
    isClickOnYBottomEdge?: boolean
  ) => {
    if (heatmapData) {
      const timeForPixel =
        (heatmapData.endTime - heatmapData.startTime) / heatmapW;
      const valueForPixel =
        (heatmapData.maxValue - heatmapData.minValue) / HEATMAP_HEIGHT;

      const { smaller: smallerX, bigger: biggerX } = sortCoordinates(
        xStart,
        xEnd
      );
      const { smaller: smallerY, bigger: biggerY } = sortCoordinates(
        HEATMAP_HEIGHT - yStart,
        HEATMAP_HEIGHT - yEnd
      );

      // to fetch correct profiles when clicking on edge cells
      const selectionMinValue = Math.round(
        valueForPixel * smallerY + heatmapData.minValue
      );

      dispatch(
        fetchSelectionProfile({
          from,
          until,
          query,
          heatmapTimeBuckets: DEFAULT_HEATMAP_PARAMS.heatmapTimeBuckets,
          heatmapValueBuckets: DEFAULT_HEATMAP_PARAMS.heatmapValueBuckets,
          selectionStartTime: timeForPixel * smallerX + heatmapData.startTime,
          selectionEndTime: timeForPixel * biggerX + heatmapData.startTime,
          selectionMinValue: isClickOnYBottomEdge
            ? selectionMinValue - 1
            : selectionMinValue,
          selectionMaxValue: Math.round(
            valueForPixel * biggerY + heatmapData.minValue
          ),
        })
      );
    }
  };

  return { fetchProfile };
};

const sortCoordinates = (
  v1: number,
  v2: number
): { smaller: number; bigger: number } => {
  const isFirstBigger = v1 > v2;

  return {
    smaller: isFirstBigger ? v2 : v1,
    bigger: isFirstBigger ? v1 : v2,
  };
};
