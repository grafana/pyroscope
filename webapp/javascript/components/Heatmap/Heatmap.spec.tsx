import React from 'react';
import { render, screen, within } from '@testing-library/react';

import { Heatmap } from '.';
import { heatmapMockData } from '../../services/exemplarsTestData';

jest.mock('./useHeatmapSelection.hook', () => ({
  ...jest.requireActual('./useHeatmapSelection.hook'),
  useHeatmapSelection: () => ({
    selectedCoordinates: { start: null, end: null },
    selectedAreaToHeatmapRatio: 1,
    hasSelectedArea: false,
  }),
}));

const renderHeatmap = () => {
  render(
    <Heatmap
      heatmap={heatmapMockData}
      onSelection={() => ({})}
      timezone="utc"
    />
  );
};

describe('Component: Heatmap', () => {
  it('should have all main elements', () => {
    renderHeatmap();

    expect(screen.getByTestId('heatmap-container')).toBeInTheDocument();
    expect(screen.getByTestId('y-axis')).toBeInTheDocument();
    expect(screen.getByTestId('x-axis')).toBeInTheDocument();
    expect(screen.getByRole('img')).toBeInTheDocument();
    expect(screen.getByTestId('selection-canvas')).toBeInTheDocument();
    expect(screen.getByTestId('color-scale')).toBeInTheDocument();
  });

  it('should have correct x-axis', () => {
    renderHeatmap();

    const xAxisTicks = within(screen.getByTestId('x-axis')).getAllByRole(
      'textbox'
    );
    expect(xAxisTicks).toHaveLength(8);
  });

  it('should have correct y-axis', () => {
    renderHeatmap();

    const xAxisTicks = within(screen.getByTestId('y-axis')).getAllByRole(
      'textbox'
    );
    expect(xAxisTicks).toHaveLength(6);
  });

  it('should have correct color scale', () => {
    renderHeatmap();

    const [maxTextEl, midTextEl, minTextEl] = within(
      screen.getByTestId('color-scale')
    ).getAllByRole('textbox');
    expect(maxTextEl.textContent).toBe(heatmapMockData.maxDepth.toString());
    expect(midTextEl.textContent).toBe('11539');
    expect(minTextEl.textContent).toBe(heatmapMockData.minDepth.toString());
  });
});
