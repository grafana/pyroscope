import React from 'react';
import { render, screen } from '@testing-library/react';

import Heatmap from './Heatmap';

jest.mock('./useHeatmapSelection.hook', () => ({
  ...jest.requireActual('./useHeatmapSelection.hook'),
  useHeatmapSelection: () => ({
    selectedCoordinates: { start: null, end: null },
    selectedAreaToHeatmapRatio: 1,
    hasSelectedArea: false,
  }),
}));

describe('Component: Heatmap', () => {
  it('should have correct structure', () => {
    render(<Heatmap />);

    expect(screen.getByTestId('heatmap-container')).toBeInTheDocument();
  });
});
