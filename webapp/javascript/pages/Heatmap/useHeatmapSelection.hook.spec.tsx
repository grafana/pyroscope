import type { RefObject } from 'react';
import { renderHook } from '@testing-library/react-hooks';

import { useHeatmapSelection } from './useHeatmapSelection.hook';
import { exemplarsQueryHeatmap } from '../../services/exemplarsTestData';

const canvasEl = document.createElement('canvasEl');
const canvasRef = { current: canvasEl } as RefObject<HTMLCanvasElement>;

describe('Hook: useHeatmapSelection', () => {
  const render = () =>
    renderHook(() =>
      useHeatmapSelection({
        timeBuckets: exemplarsQueryHeatmap.timeBuckets,
        valueBuckets: exemplarsQueryHeatmap.valueBuckets,
        values: exemplarsQueryHeatmap.values,
        canvasRef,
        heatmapW: 1234,
        heatmapH: 123,
      })
    ).result;

  it('should return initial selection values', () => {
    const { current } = render();

    expect(current).toMatchObject({
      selectedCoordinates: { start: null, end: null },
      selectedAreaToHeatmapRatio: 1,
      hasSelectedArea: false,
      resetSelection: expect.any(Function),
    });
  });
});
