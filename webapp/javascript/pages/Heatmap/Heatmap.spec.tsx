import React from 'react';
import { render, screen, within } from '@testing-library/react';

import { Heatmap } from './Heatmap';
// import { exemplarsQueryHeatmap } from '../../services/exemplarsTestData';

jest.mock('./useHeatmapSelection.hook', () => ({
  ...jest.requireActual('./useHeatmapSelection.hook'),
  useHeatmapSelection: () => ({
    selectedCoordinates: { start: null, end: null },
    selectedAreaToHeatmapRatio: 1,
    hasSelectedArea: false,
  }),
}));

// TODO(dogfrogfog): refactor
describe.skip('Component: Heatmap', () => {
  it('should have all main elements', () => {
    render(<Heatmap />);

    expect(screen.getByTestId('heatmap-container')).toBeInTheDocument();
    expect(screen.getByTestId('y-axis')).toBeInTheDocument();
    expect(screen.getByTestId('x-axis')).toBeInTheDocument();
    expect(screen.getByRole('img')).toBeInTheDocument();
    expect(screen.getByTestId('selection-canvas')).toBeInTheDocument();
    expect(screen.getByTestId('color-scale')).toBeInTheDocument();
  });

  it('should have correct x-axis', () => {
    render(<Heatmap />);

    const xAxisTicks = within(screen.getByTestId('x-axis')).getAllByRole(
      'textbox'
    );
    expect(xAxisTicks).toHaveLength(8);
  });

  it('should have correct y-axis', () => {
    render(<Heatmap />);

    const xAxisTicks = within(screen.getByTestId('y-axis')).getAllByRole(
      'textbox'
    );
    expect(xAxisTicks).toHaveLength(6);
  });

  it('should have correct color scale', () => {
    render(<Heatmap />);

    const [minTextEl, maxTextEl] = within(
      screen.getByTestId('color-scale')
    ).getAllByRole('textbox');
    // expect(minTextEl.textContent).toBe(`${exemplarsQueryHeatmap.minDepth - 1}`);
    // expect(maxTextEl.textContent).toBe(`${exemplarsQueryHeatmap.maxDepth}`);
  });
});
